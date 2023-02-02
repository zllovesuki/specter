package promise

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestConcurrent(t *testing.T) {
	as := require.New(t)

	wait := []time.Duration{
		time.Second,
		time.Second * 2,
		time.Second * 3,
	}

	jobs := make([]func(context.Context) (time.Duration, error), 0)
	for _, d := range wait {
		d := d
		jobs = append(jobs, func(ctx context.Context) (time.Duration, error) {
			time.Sleep(d)
			return d, nil
		})
	}

	jobCtx, jobCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer jobCancel()

	start := time.Now()
	results, errors := All(jobCtx, jobs...)
	end := time.Now()

	for i, err := range errors {
		as.NoError(err)
		as.Equal(results[i], wait[i])
	}

	as.Less(end.Sub(start), wait[0]+wait[1]+wait[2])
}

func TestConcurrentError(t *testing.T) {
	as := require.New(t)

	wait := []time.Duration{
		time.Second,
		time.Second * 2,
		time.Second * 3,
	}

	jobs := make([]func(context.Context) (time.Duration, error), 0)
	for i, d := range wait {
		i := i
		d := d
		jobs = append(jobs, func(ctx context.Context) (time.Duration, error) {
			time.Sleep(d)
			if i == 1 {
				return d, fmt.Errorf("error")
			}
			return d, nil
		})
	}

	jobCtx, jobCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer jobCancel()

	start := time.Now()
	results, errors := All(jobCtx, jobs...)
	end := time.Now()

	for i, err := range errors {
		if i == 1 {
			as.Error(err)
			as.Equal(results[i], time.Duration(0))
		} else {
			as.NoError(err)
			as.Equal(results[i], wait[i])
		}
	}

	as.Less(end.Sub(start), wait[0]+wait[1]+wait[2])
}
