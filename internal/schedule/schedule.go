package schedule

import (
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"time"

	"github.com/appbip/appbip/internal/people"

	_ "time/tzdata"
)

type Job struct {
	Label  string
	Action string
	When   time.Time
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

	fmt.Printf("appbip schedule (Mon–Fri, %s) — 2 check-in random 07:50:00–08:05:00, 1 check-out random 17:00:00–17:05:00 — Ctrl+C to stop\n", loc.String())

	var (
		mu       sync.Mutex
		planDate time.Time
		jobs     []Job
	)

	for {
		now := time.Now().In(loc)
		today := dateOnly(now)
		if !planDate.Equal(today) {
			planDate = today
			jobs = planJobs(now)
			printPlan(now, jobs)
		}

		next := nextJob(now, jobs)
		if next == nil {
			wake := nextWeekday(now)
			fmt.Printf("no remaining jobs today — sleeping until %s\n", wake.Format("2006-01-02 15:04:05"))
			if err := sleepUntil(wake); err != nil {
				return err
			}
			continue
		}

		fmt.Printf("next: %s at %s\n", next.Label, next.When.Format("15:04:05"))
		if err := sleepUntil(next.When); err != nil {
			return err
		}
		mu.Lock()
		runJob(runner, *next)
		mu.Unlock()
	}
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func planJobs(now time.Time) []Job {
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return nil
	}
	// Random xuống từng giây trong khoảng [start, end] inclusive.
	in := pickTwo(now, 7, 50, 0, 8, 5, 0, 90*time.Second)
	out := pickOne(now, 17, 0, 0, 17, 5, 0)
	return []Job{
		{Label: "check-in lần 1", Action: "check-in", When: in[0]},
		{Label: "check-in lần 2", Action: "check-in", When: in[1]},
		{Label: "check-out", Action: "check-out", When: out},
	}
}

// pickTwo picks two distinct random times (hour:min:sec) in [h1:m1:s1, h2:m2:s2],
// ordered ascending, at least minGap apart.
func pickTwo(day time.Time, h1, m1, s1, h2, m2, s2 int, minGap time.Duration) []time.Time {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), h1, m1, s1, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), h2, m2, s2, 0, loc)
	span := int(end.Sub(start).Seconds())
	if span < 0 {
		span = 0
	}
	for try := 0; try < 200; try++ {
		a := start.Add(time.Duration(rand.IntN(span+1)) * time.Second)
		b := start.Add(time.Duration(rand.IntN(span+1)) * time.Second)
		if a.After(b) {
			a, b = b, a
		}
		if b.Sub(a) >= minGap {
			return []time.Time{a, b}
		}
	}
	second := start.Add(minGap)
	if second.After(end) {
		second = end
	}
	return []time.Time{start, second}
}

// pickOne picks one random time (hour:min:sec) in [h1:m1:s1, h2:m2:s2].
func pickOne(day time.Time, h1, m1, s1, h2, m2, s2 int) time.Time {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), h1, m1, s1, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), h2, m2, s2, 0, loc)
	span := int(end.Sub(start).Seconds())
	if span < 0 {
		span = 0
	}
	return start.Add(time.Duration(rand.IntN(span+1)) * time.Second)
}

func nextJob(now time.Time, jobs []Job) *Job {
	for i := range jobs {
		if jobs[i].When.After(now) {
			j := jobs[i]
			return &j
		}
	}
	return nil
}

func printPlan(now time.Time, jobs []Job) {
	if len(jobs) == 0 {
		fmt.Printf("=== %s (%s) weekend — no jobs ===\n", now.Format("2006-01-02"), now.Weekday())
		return
	}
	fmt.Printf("=== %s (%s) ===\n", now.Format("2006-01-02"), now.Weekday())
	for _, j := range jobs {
		mark := "pending"
		if !j.When.After(now) {
			mark = "skipped (already passed)"
		}
		fmt.Printf("  %-18s %s  %s\n", j.Label, j.When.Format("15:04:05"), mark)
	}
}

func nextWeekday(now time.Time) time.Time {
	d := now.AddDate(0, 0, 1)
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, now.Location())
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

func sleepUntil(t time.Time) error {
	d := time.Until(t)
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	<-timer.C
	return nil
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

var _ Runner = (*people.Runner)(nil)
