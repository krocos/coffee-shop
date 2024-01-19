package testingexample

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type UnitTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite

	env *testsuite.TestWorkflowEnvironment
}

func (s *UnitTestSuite) SetupTest() {
	s.env = s.NewTestWorkflowEnvironment()
}

func (s *UnitTestSuite) TearDownTest() {
	s.env.AssertExpectations(s.T())
}

func TestSumWorkflowExample(t *testing.T) {
	suite.Run(t, new(UnitTestSuite))
}

func (s *UnitTestSuite) TestExpectedSum() {
	var (
		d1, d2 = 4, 6
		client *SumServiceClient
	)

	s.env.RegisterActivity(client)

	s.env.OnActivity(client.SumThis, mock.Anything, d1, d2).Return(10, nil)
	s.env.ExecuteWorkflow(SumWorkflowExample, d1, d2)

	s.True(s.env.IsWorkflowCompleted())
	s.NoError(s.env.GetWorkflowError())

	var result int
	s.NoError(s.env.GetWorkflowResult(&result))
	s.Equal(10, result)

	assert.True(s.T(), true)
}

func TestSumServiceClient_SumThis(t *testing.T) {
	type args struct {
		ctx context.Context
		d1  int
		d2  int
	}
	tests := []struct {
		name    string
		args    args
		want    int
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "4 + 6 = 10",
			args: args{
				ctx: context.Background(),
				d1:  4,
				d2:  6,
			},
			want:    10,
			wantErr: assert.NoError,
		},
		{
			name: "-7 + 7 = 0",
			args: args{
				ctx: context.Background(),
				d1:  -7,
				d2:  7,
			},
			want:    0,
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &SumServiceClient{}
			got, err := c.SumThis(tt.args.ctx, tt.args.d1, tt.args.d2)
			if !tt.wantErr(t, err, fmt.Sprintf("SumThis(%v, %v, %v)", tt.args.ctx, tt.args.d1, tt.args.d2)) {
				return
			}
			assert.Equalf(t, tt.want, got, "SumThis(%v, %v, %v)", tt.args.ctx, tt.args.d1, tt.args.d2)
		})
	}
}
