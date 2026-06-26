package schedule

import (
	"fmt"
	"os"
	"time"

	"github.com/appbip/appbip/internal/people"
	"github.com/robfig/cron/v3"
)

// Default jobs: check-in x2 buổi sáng, check-out x2 buổi chiều (giờ VN).
var DefaultJobs = []Job{
	{Label: "check-in lần 1", Cron: "50 7 * * *", Action: "check-in"},
	{Label: "check-in lần 2", Cron: "5 8 * * *", Action: "check-in"},
	{Label: "check-out lần 1", Cron: "0 17 * * *", Action: "check-out"},
	{Label: "check-out lần 2", Cron: "0 18 * * *", Action: "check-out"},
}

type Job struct {
	Label  string
	Cron   string
	Action string // check-in | check-out
}

type Runner interface {
	CheckIn() error
	CheckOut() error
}

func Run(runner Runner) error {
	loc, err := time.LoadLocation(timezone())
	if err != nil {
		return fmt.Errorf("load timezone: %w", err)
	}

	c := cron.New(cron.WithLocation(loc))
	for _, job := range DefaultJobs {
		j := job
		_, err := c.AddFunc(j.Cron, func() {
			runJob(runner, j)
		})
		if err != nil {
			return fmt.Errorf("schedule %s: %w", j.Label, err)
		}
		fmt.Printf("scheduled %s at cron %q (%s)\n", j.Label, j.Cron, loc.String())
	}

	c.Start()
	fmt.Println("appbip schedule running — Ctrl+C to stop")
	select {}
}

func runJob(runner Runner, job Job) {
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("\n=== [%s] %s ===\n", now, job.Label)
	var err error
	switch job.Action {
	case "check-in":
		err = runner.CheckIn()
	case "check-out":
		err = runner.CheckOut()
	default:
		err = fmt.Errorf("unknown action %q", job.Action)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] job aborted (cleanup applied) — waiting for next schedule\n", job.Label)
		return
	}
	fmt.Printf("=== [%s] completed ===\n", job.Label)
}

func timezone() string {
	if tz := os.Getenv("APPBIP_TZ"); tz != "" {
		return tz
	}
	return "Asia/Ho_Chi_Minh"
}

// Ensure people.Runner satisfies Runner at compile time.
var _ Runner = (*people.Runner)(nil)
