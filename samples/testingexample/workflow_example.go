package testingexample

import (
	"context"
	"time"

	"go.temporal.io/sdk/workflow"
)

func SumWorkflowExample(ctx workflow.Context, d1, d2 int) (int, error) {
	ao := workflow.ActivityOptions{StartToCloseTimeout: time.Minute}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var sumClient *SumServiceClient

	var result int
	if err := workflow.ExecuteActivity(ctx, sumClient.SumThis, d1, d2).Get(ctx, &result); err != nil {
		return 0, err
	}

	return result, nil
}

type SumServiceClient struct{}

func (c *SumServiceClient) SumThis(ctx context.Context, d1, d2 int) (int, error) {
	return d1 + d2, nil
}
