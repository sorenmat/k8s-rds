// Code generated by private/model/cli/gen-api/main.go. DO NOT EDIT.

// Package codepipelineiface provides an interface to enable mocking the AWS CodePipeline service client
// for testing your code.
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters.
package codepipelineiface

import (
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
)

// CodePipelineAPI provides an interface to enable mocking the
// codepipeline.CodePipeline service client's API operation,
// paginators, and waiters. This make unit testing your code that calls out
// to the SDK's service client's calls easier.
//
// The best way to use this interface is so the SDK's service client's calls
// can be stubbed out for unit testing your code with the SDK without needing
// to inject custom request handlers into the SDK's request pipeline.
//
//    // myFunc uses an SDK service client to make a request to
//    // AWS CodePipeline.
//    func myFunc(svc codepipelineiface.CodePipelineAPI) bool {
//        // Make svc.AcknowledgeJob request
//    }
//
//    func main() {
//        cfg, err := external.LoadDefaultAWSConfig()
//        if err != nil {
//            panic("failed to load config, " + err.Error())
//        }
//
//        svc := codepipeline.New(cfg)
//
//        myFunc(svc)
//    }
//
// In your _test.go file:
//
//    // Define a mock struct to be used in your unit tests of myFunc.
//    type mockCodePipelineClient struct {
//        codepipelineiface.CodePipelineAPI
//    }
//    func (m *mockCodePipelineClient) AcknowledgeJob(input *codepipeline.AcknowledgeJobInput) (*codepipeline.AcknowledgeJobOutput, error) {
//        // mock response/functionality
//    }
//
//    func TestMyFunc(t *testing.T) {
//        // Setup Test
//        mockSvc := &mockCodePipelineClient{}
//
//        myfunc(mockSvc)
//
//        // Verify myFunc's functionality
//    }
//
// It is important to note that this interface will have breaking changes
// when the service model is updated and adds new API operations, paginators,
// and waiters. Its suggested to use the pattern above for testing, or using
// tooling to generate mocks to satisfy the interfaces.
type CodePipelineAPI interface {
	AcknowledgeJobRequest(*codepipeline.AcknowledgeJobInput) codepipeline.AcknowledgeJobRequest

	AcknowledgeThirdPartyJobRequest(*codepipeline.AcknowledgeThirdPartyJobInput) codepipeline.AcknowledgeThirdPartyJobRequest

	CreateCustomActionTypeRequest(*codepipeline.CreateCustomActionTypeInput) codepipeline.CreateCustomActionTypeRequest

	CreatePipelineRequest(*codepipeline.CreatePipelineInput) codepipeline.CreatePipelineRequest

	DeleteCustomActionTypeRequest(*codepipeline.DeleteCustomActionTypeInput) codepipeline.DeleteCustomActionTypeRequest

	DeletePipelineRequest(*codepipeline.DeletePipelineInput) codepipeline.DeletePipelineRequest

	DeleteWebhookRequest(*codepipeline.DeleteWebhookInput) codepipeline.DeleteWebhookRequest

	DeregisterWebhookWithThirdPartyRequest(*codepipeline.DeregisterWebhookWithThirdPartyInput) codepipeline.DeregisterWebhookWithThirdPartyRequest

	DisableStageTransitionRequest(*codepipeline.DisableStageTransitionInput) codepipeline.DisableStageTransitionRequest

	EnableStageTransitionRequest(*codepipeline.EnableStageTransitionInput) codepipeline.EnableStageTransitionRequest

	GetJobDetailsRequest(*codepipeline.GetJobDetailsInput) codepipeline.GetJobDetailsRequest

	GetPipelineRequest(*codepipeline.GetPipelineInput) codepipeline.GetPipelineRequest

	GetPipelineExecutionRequest(*codepipeline.GetPipelineExecutionInput) codepipeline.GetPipelineExecutionRequest

	GetPipelineStateRequest(*codepipeline.GetPipelineStateInput) codepipeline.GetPipelineStateRequest

	GetThirdPartyJobDetailsRequest(*codepipeline.GetThirdPartyJobDetailsInput) codepipeline.GetThirdPartyJobDetailsRequest

	ListActionTypesRequest(*codepipeline.ListActionTypesInput) codepipeline.ListActionTypesRequest

	ListPipelineExecutionsRequest(*codepipeline.ListPipelineExecutionsInput) codepipeline.ListPipelineExecutionsRequest

	ListPipelinesRequest(*codepipeline.ListPipelinesInput) codepipeline.ListPipelinesRequest

	ListWebhooksRequest(*codepipeline.ListWebhooksInput) codepipeline.ListWebhooksRequest

	PollForJobsRequest(*codepipeline.PollForJobsInput) codepipeline.PollForJobsRequest

	PollForThirdPartyJobsRequest(*codepipeline.PollForThirdPartyJobsInput) codepipeline.PollForThirdPartyJobsRequest

	PutActionRevisionRequest(*codepipeline.PutActionRevisionInput) codepipeline.PutActionRevisionRequest

	PutApprovalResultRequest(*codepipeline.PutApprovalResultInput) codepipeline.PutApprovalResultRequest

	PutJobFailureResultRequest(*codepipeline.PutJobFailureResultInput) codepipeline.PutJobFailureResultRequest

	PutJobSuccessResultRequest(*codepipeline.PutJobSuccessResultInput) codepipeline.PutJobSuccessResultRequest

	PutThirdPartyJobFailureResultRequest(*codepipeline.PutThirdPartyJobFailureResultInput) codepipeline.PutThirdPartyJobFailureResultRequest

	PutThirdPartyJobSuccessResultRequest(*codepipeline.PutThirdPartyJobSuccessResultInput) codepipeline.PutThirdPartyJobSuccessResultRequest

	PutWebhookRequest(*codepipeline.PutWebhookInput) codepipeline.PutWebhookRequest

	RegisterWebhookWithThirdPartyRequest(*codepipeline.RegisterWebhookWithThirdPartyInput) codepipeline.RegisterWebhookWithThirdPartyRequest

	RetryStageExecutionRequest(*codepipeline.RetryStageExecutionInput) codepipeline.RetryStageExecutionRequest

	StartPipelineExecutionRequest(*codepipeline.StartPipelineExecutionInput) codepipeline.StartPipelineExecutionRequest

	UpdatePipelineRequest(*codepipeline.UpdatePipelineInput) codepipeline.UpdatePipelineRequest
}

var _ CodePipelineAPI = (*codepipeline.CodePipeline)(nil)