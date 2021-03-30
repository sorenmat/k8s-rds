package rds

import (
	"context"
	"fmt"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/provider"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type RDS struct {
	EC2             *ec2.Client
	Config          aws.Config
	Subnets         []string
	SecurityGroups  []string
	VpcId           string
	ServiceProvider provider.ServiceProvider
}

func New(ctx context.Context, db *crd.Database, kc *kubernetes.Clientset) (*RDS, error) {
	cfg, err := ec2config(ctx, kc)
	if err != nil {
		log.Fatal("unable to create a client for EC2 ", err)
	}

	ec2client := ec2.NewFromConfig(*cfg)

	nodeInfo, err := describeNodeEC2Instance(ctx, kc, ec2client)
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable AWS metadata")
	}
	vpcId := *nodeInfo.Reservations[0].Instances[0].VpcId

	log.Println("trying to get subnets")
	subnets, err := getSubnets(ctx, nodeInfo, ec2client, db.Spec.PubliclyAccessible)
	if err != nil {
		return nil, fmt.Errorf("unable to get subnets from instance: %v", err)

	}

	log.Println("trying to get security groups")
	sgs, err := getSGS(ctx, kc, ec2client)
	if err != nil {
		return nil, fmt.Errorf("unable to get security groups from instance: %v", err)

	}

	r := RDS{
		EC2:            ec2client,
		Config:         *cfg,
		Subnets:        subnets,
		SecurityGroups: sgs,
		VpcId:          vpcId,
	}
	return &r, nil
}

// CreateDatabase creates a database from the CRD database object, is also ensures that the correct
// subnets are created for the database so we can access it
func (r *RDS) CreateDatabase(ctx context.Context, db *crd.Database) (string, error) {
	// Ensure that the subnets for the DB is create or updated
	log.Println("Trying to find the correct subnets")
	subnetName, err := r.ensureSubnets(ctx, db)
	if err != nil {
		return "", err
	}

	log.Printf("getting secret: Name: %v Key: %v \n", db.Spec.Password.Name, db.Spec.Password.Key)
	pw, err := r.GetSecret(db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
	if err != nil {
		return "", err
	}
	input := convertSpecToInput(db, subnetName, r.SecurityGroups, pw)

	// search for the instance
	log.Printf("Trying to find db instance %v\n", db.Spec.DBName)
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: input.DBInstanceIdentifier}

	_, err = r.rdsclient().DescribeDBInstances(ctx, k)
	if err != nil && err.Error() != new(rdstypes.DBInstanceNotFoundFault).Error() {
		log.Printf("DB instance %v not found trying to create it\n", db.Spec.DBName)
		// seems like we didn't find a database with this name, let's create on
		_, err := r.rdsclient().CreateDBInstance(ctx, input)
		if err != nil {
			return "", errors.Wrap(err, "CreateDBInstance")
		}
	} else if err != nil {
		return "", errors.Wrap(err, fmt.Sprintf("wasn't able to describe the db instance with id %v", input.DBInstanceIdentifier))
	}
	log.Printf("Waiting for db instance %v to become available\n", *input.DBInstanceIdentifier)

	time.Sleep(5 * time.Second)

	// Get the newly created database so we can get the endpoint
	dbHostname, err := getEndpoint(ctx, input.DBInstanceIdentifier, r.rdsclient())
	if err != nil {
		return "", err
	}
	return dbHostname, nil
}

// ensureSubnets is ensuring that we have created or updated the subnet according to the data from the CRD object
func (r *RDS) ensureSubnets(ctx context.Context, db *crd.Database) (string, error) {
	if len(r.Subnets) == 0 {
		log.Println("Error: unable to continue due to lack of subnets, perhaps we couldn't lookup the subnets")
	}
	subnetDescription := "RDS Subnet Group for VPC: " + r.VpcId
	subnetName := "db-subnetgroup-" + r.VpcId

	svc := r.rdsclient()

	sf := &rds.DescribeDBSubnetGroupsInput{DBSubnetGroupName: aws.String(subnetName)}
	_, err := svc.DescribeDBSubnetGroups(ctx, sf)
	log.Println("Subnets:", r.Subnets)
	if err != nil {
		// assume we didn't find it..
		subnet := &rds.CreateDBSubnetGroupInput{
			DBSubnetGroupDescription: aws.String(subnetDescription),
			DBSubnetGroupName:        aws.String(subnetName),
			SubnetIds:                r.Subnets,
			Tags:                     []rdstypes.Tag{{Key: aws.String("Warning"), Value: aws.String("Managed by k8s-rds.")}},
		}
		_, err := svc.CreateDBSubnetGroup(ctx, subnet)
		if err != nil {
			return "", errors.Wrap(err, "CreateDBSubnetGroup")
		}
	} else {
		log.Printf("Moving on seems like %v exsits", subnetName)
	}
	return subnetName, nil
}

func getEndpoint(ctx context.Context, dbName *string, svc *rds.Client) (string, error) {
	k := &rds.DescribeDBInstancesInput{DBInstanceIdentifier: dbName}

	instance, err := svc.DescribeDBInstances(ctx, k)
	if err != nil || len(instance.DBInstances) == 0 {
		return "", fmt.Errorf("wasn't able to describe the db instance with id %v", dbName)
	}
	rdsdb := instance.DBInstances[0]

	dbHostname := *rdsdb.Endpoint.Address
	return dbHostname, nil
}

func (r *RDS) DeleteDatabase(ctx context.Context, db *crd.Database) error {
	if db.Spec.DeleteProtection {
		log.Printf("Trying to delete a %v in %v which is a deleted protected database", db.Name, db.Namespace)
		return nil
	}
	// delete the database instance
	svc := r.rdsclient()

	_, err := svc.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
		DBInstanceIdentifier: aws.String(dbidentifier(db)),
		SkipFinalSnapshot:    true,
	})

	if err != nil {
		err := errors.Wrap(err, fmt.Sprintf("unable to delete database %v", db.Spec.DBName))
		log.Println(err)
		return err
	}

	log.Printf("Waiting for db instance %v to be deleted\n", db.Spec.DBName)
	time.Sleep(5 * time.Second)

	// delete the subnet group attached to the instance
	subnetName := db.Name + "-subnet-" + db.Namespace
	_, err = svc.DeleteDBSubnetGroup(ctx, &rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: aws.String(subnetName)})
	if err != nil {
		e := errors.Wrap(err, fmt.Sprintf("unable to delete subnet %v", subnetName))
		log.Println(e)
		return e
	} else {
		log.Println("Deleted DBSubnet group: ", subnetName)
	}
	return nil
}

func (r *RDS) rdsclient() *rds.Client {
	return rds.NewFromConfig(r.Config)
}
func dbidentifier(v *crd.Database) string {
	return v.Name + "-" + v.Namespace
}

const (
	maxTagLengthAllowed = 255
	tagRegexp           = `^kube.*$`
)

func toTags(annotations, labels map[string]string) []rdstypes.Tag {
	var tags []rdstypes.Tag
	r := regexp.MustCompile(tagRegexp)

	for k, v := range annotations {
		if len(k) > maxTagLengthAllowed || len(v) > maxTagLengthAllowed ||
			r.Match([]byte(k)) {
			log.Printf("WARNING: Not Adding annotation KV to tags: %v %v", k, v)
			continue
		}

		tags = append(tags, rdstypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}
	for k, v := range labels {
		if len(k) > maxTagLengthAllowed || len(v) > maxTagLengthAllowed {
			log.Printf("WARNING: Not Adding CRD labels KV to tags: %v %v", k, v)
			continue
		}

		tags = append(tags, rdstypes.Tag{Key: aws.String(k), Value: aws.String(v)})
	}

	return tags
}

func gettags(db *crd.Database) []rdstypes.Tag {
	var tags []rdstypes.Tag
	if db.Spec.Tags == "" {
		return tags
	}
	for _, v := range strings.Split(db.Spec.Tags, ",") {
		kv := strings.Split(v, "=")

		tags = append(tags, rdstypes.Tag{Key: aws.String(strings.TrimSpace(kv[0])), Value: aws.String(strings.TrimSpace(kv[1]))})
	}
	return tags
}

func convertSpecToInput(v *crd.Database, subnetName string, securityGroups []string, password string) *rds.CreateDBInstanceInput {
	tags := toTags(v.Annotations, v.Labels)
	tags = append(tags, gettags(v)...)

	input := &rds.CreateDBInstanceInput{
		DBName:                aws.String(v.Spec.DBName),
		AllocatedStorage:      aws.Int32(int32(v.Spec.Size)),
		MaxAllocatedStorage:   aws.Int32(int32(v.Spec.MaxAllocatedSize)),
		DBInstanceClass:       aws.String(v.Spec.Class),
		DBInstanceIdentifier:  aws.String(dbidentifier(v)),
		VpcSecurityGroupIds:   securityGroups,
		Engine:                aws.String(v.Spec.Engine),
		MasterUserPassword:    aws.String(password),
		MasterUsername:        aws.String(v.Spec.Username),
		DBSubnetGroupName:     aws.String(subnetName),
		PubliclyAccessible:    aws.Bool(v.Spec.PubliclyAccessible),
		MultiAZ:               aws.Bool(v.Spec.MultiAZ),
		StorageEncrypted:      aws.Bool(v.Spec.StorageEncrypted),
		BackupRetentionPeriod: aws.Int32(int32(v.Spec.BackupRetentionPeriod)),
		DeletionProtection:    aws.Bool(v.Spec.DeleteProtection),
		Tags:                  tags,
	}
	if v.Spec.Version != "" {
		input.EngineVersion = aws.String(v.Spec.Version)
	}
	if v.Spec.StorageType != "" {
		input.StorageType = aws.String(v.Spec.StorageType)
	}
	if v.Spec.Iops > 0 {
		input.Iops = aws.Int32(int32(v.Spec.Iops))
	}
	return input
}

//DescribeInstancesResponse
// describeNodeEC2Instance returns the AWS Metadata for the firt Node from the cluster
func describeNodeEC2Instance(ctx context.Context, kubectl *kubernetes.Clientset, svc *ec2.Client) (*ec2.DescribeInstancesOutput, error) {
	nodes, err := kubectl.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) == 0 {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}

	// take the first one, we assume that all nodes are created in the same VPC
	name = getIDFromProvider(nodes.Items[0].Spec.ProviderID)

	log.Printf("Taking subnets from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	//DescribeInstancesRequest
	nodeInfo, err := svc.DescribeInstances(ctx, params)
	//nodeInfo, err := req.Send(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	if len(nodeInfo.Reservations) == 0 {
		log.Println(err)
		return nil, fmt.Errorf("unable to describe AWS instance")
	}

	return nodeInfo, nil
}

// getSubnets returns a list of subnets within the VPC from the Kubernetes Node.
func getSubnets(ctx context.Context, nodeInfo *ec2.DescribeInstancesOutput, svc *ec2.Client, public bool) ([]string, error) {
	var result []string
	vpcID := nodeInfo.Reservations[0].Instances[0].VpcId
	for _, v := range nodeInfo.Reservations[0].Instances[0].SecurityGroups {
		log.Println("Security groupid: ", *v.GroupId)
	}
	log.Printf("Found VPC %v will search for subnet in that VPC\n", *vpcID)

	//DescribeSubnetsRequest
	subnets, err := svc.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{Filters: []ec2types.Filter{{Name: aws.String("vpc-id"), Values: []string{*vpcID}}}})
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unable to describe subnet in VPC %v", *vpcID))
	}
	for _, sn := range subnets.Subnets {
		if sn.MapPublicIpOnLaunch == public {
			result = append(result, *sn.SubnetId)
		} else {
			log.Printf("Skipping subnet %v since it's public state was %v and we were looking for %v\n", *sn.SubnetId, sn.MapPublicIpOnLaunch, public)
		}
	}

	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func getIDFromProvider(x string) string {
	pos := strings.LastIndex(x, "/") + 1
	name := x[pos:]
	return name
}
func getSGS(ctx context.Context, kubectl *kubernetes.Clientset, svc *ec2.Client) ([]string, error) {

	nodes, err := kubectl.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = getIDFromProvider(nodes.Items[0].Spec.ProviderID)
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Printf("Taking security groups from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2types.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	res, err := svc.DescribeInstances(ctx, params)
	//res, err := req.Send(context.Background())
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	log.Println("got instance response")

	var result []string
	if len(res.Reservations) >= 1 {
		for _, v := range res.Reservations[0].Instances[0].SecurityGroups {
			log.Println("Security groupid: ", *v.GroupId)
			result = append(result, *v.GroupId)
		}
	}

	log.Printf("Found the follwing security groups: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func ec2config(ctx context.Context, kubectl *kubernetes.Clientset) (*aws.Config, error) {
	nodes, err := kubectl.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""
	region := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = getIDFromProvider(nodes.Items[0].Spec.ProviderID)
		region = nodes.Items[0].Labels["failure-domain.beta.kubernetes.io/region"]
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Printf("Found node with ID: %v in region %v", name, region)

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	// Set the AWS Region that the service clients should use
	cfg.Region = region
	return &cfg, nil
}
