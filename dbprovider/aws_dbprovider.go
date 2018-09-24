package dbprovider

import (
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/sorenmat/k8s-rds/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/pkg/errors"
	"github.com/sorenmat/k8s-rds/crd"
	"github.com/sorenmat/k8s-rds/rds"
	"k8s.io/client-go/kubernetes"
	"log"
)

type AWSDBProvider struct {
	Client 	*ec2.EC2
}


// getSubnets returns a list of subnets that the RDS instance should be attached to
// We do this by finding a node in the cluster, take the VPC id from that node a list
// the security groups in the VPC
func getSubnets(kubectl *kubernetes.Clientset, svc *ec2.EC2, public bool) ([]string, error) {
	nodes, err := kubectl.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = nodes.Items[0].Spec.ExternalID
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Printf("Taking subnets from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	req := svc.DescribeInstancesRequest(params)
	res, err := req.Send()
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	log.Println("got instance response")

	var result []string
	if len(res.Reservations) >= 1 {
		vpcID := res.Reservations[0].Instances[0].VpcId
		for _, v := range res.Reservations[0].Instances[0].SecurityGroups {
			log.Println("Security groupid: ", *v.GroupId)
		}
		log.Printf("Found VPC %v will search for subnet in that VPC\n", *vpcID)

		res := svc.DescribeSubnetsRequest(&ec2.DescribeSubnetsInput{Filters: []ec2.Filter{{Name: aws.String("vpc-id"), Values: []string{*vpcID}}}})
		subnets, err := res.Send()

		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("unable to describe subnet in VPC %v", *vpcID))
		}
		for _, sn := range subnets.Subnets {
			if *sn.MapPublicIpOnLaunch == public {
				result = append(result, *sn.SubnetId)
			} else {
				log.Printf("Skipping subnet %v since it's public state was %v and we were looking for %v\n", *sn.SubnetId, *sn.MapPublicIpOnLaunch, public)
			}
		}

	}
	log.Printf("Found the follwing subnets: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func getSGS(kubectl *kubernetes.Clientset, svc *ec2.EC2) ([]string, error) {
	nodes, err := kubectl.Core().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "unable to get nodes")
	}
	name := ""

	if len(nodes.Items) > 0 {
		// take the first one, we assume that all nodes are created in the same VPC
		name = nodes.Items[0].Spec.ExternalID
	} else {
		return nil, fmt.Errorf("unable to find any nodes in the cluster")
	}
	log.Printf("Taking security groups from node %v", name)

	params := &ec2.DescribeInstancesInput{
		Filters: []ec2.Filter{
			{
				Name: aws.String("instance-id"),
				Values: []string{
					name,
				},
			},
		},
	}
	log.Println("trying to describe instance")
	req := svc.DescribeInstancesRequest(params)
	res, err := req.Send()
	if err != nil {
		log.Println(err)
		return nil, errors.Wrap(err, "unable to describe AWS instance")
	}
	log.Println("got instance response")

	var result []string
	if len(res.Reservations) >= 1 {
		for _, v := range res.Reservations[0].Instances[0].SecurityGroups {
			fmt.Println("Security groupid: ", *v.GroupId)
			result = append(result, *v.GroupId)
		}
	}

	log.Printf("Found the follwing security groups: ")
	for _, v := range result {
		log.Printf(v + " ")
	}
	return result, nil
}

func (aws AWSDBProvider) CreateDatabase(kubectl *kubernetes.Clientset, db *crd.Database) (string, error){
	k := kube.Kube{Client: kubectl}

	password,err := k.GetSecret(db.Namespace, db.Spec.Password.Name, db.Spec.Password.Key)
	if err != nil {
		//TODO: Wrap error
		return "", err
	}

	log.Println("trying to get subnets")
	subnets, err := getSubnets(kubectl, aws.Client, db.Spec.PubliclyAccessible)
	if err != nil {
		//TODO: Wrap error
		log.Println("unable to get subnets from instance: ", err)
		return "",err
	}
	log.Println("trying to get security groups")
	sgs, err := getSGS(kubectl, aws.Client)
	if err != nil {
		//TODO: Wrap error
		log.Println("unable to get security groups from instance: ", err)
		return "",err
	}

	r := rds.RDS{EC2: aws.Client, Subnets: subnets, SecurityGroups: sgs}
	hostname, err := r.CreateDatabase(db, password)
	if err != nil {
		//TODO: Wrap error
		log.Println(err)
		return "",err
	}
	return hostname,nil
}

func (aws AWSDBProvider) DeleteDatabase(kubectl *kubernetes.Clientset, db *crd.Database) (error) {
	log.Printf("deleting database: %s \n", db.Name)
	subnets, err := getSubnets(kubectl, aws.Client, db.Spec.PubliclyAccessible)
	if err != nil {
		//TODO: Wrap error
		return err
	}
	r := rds.RDS{EC2: aws.Client, Subnets: subnets}
	r.DeleteDatabase(db)
	return nil
}

