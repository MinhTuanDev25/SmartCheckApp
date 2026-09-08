package schedule

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
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

	fmt.Printf("appbip schedule (Mon–Fri, %s) — 2 check-in 07:50–08:05, 2 check-out 17:00–17:05 (lần 2 chỉ khi lần 1 fail) — Ctrl+C to stop\n", loc.String())

	var (
		mu         sync.Mutex
		planDate   time.Time
		jobs       []Job
		succeeded  = map[string]bool{} // "check-in" / "check-out" đã OK trong ngày
	)

	for {
		now := time.Now().In(loc)
		today := dateOnly(now)
		if !planDate.Equal(today) {
			planDate = today
			succeeded = map[string]bool{}
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

		// Lần 2 (cùng action) bỏ qua nếu lần 1 trong ngày đã thành công.
		if succeeded[next.Action] {
			fmt.Printf("skip %s — %s lần trước đã thành công\n", next.Label, next.Action)
			markConsumed(&jobs, next.Label, now)
			continue
		}

		fmt.Printf("next: %s at %s\n", next.Label, next.When.Format("15:04:05"))
		if err := sleepUntil(next.When); err != nil {
			return err
		}
		mu.Lock()
		ok := runJob(runner, *next)
		if ok {
			succeeded[next.Action] = true
		}
		markConsumed(&jobs, next.Label, time.Now().In(loc))
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
	in := pickTwo(now, 7, 50, 0, 8, 5, 0, 90*time.Second)
	out := pickTwo(now, 17, 0, 0, 17, 5, 0, 90*time.Second)
	return []Job{
		{Label: "check-in lần 1", Action: "check-in", When: in[0]},
		{Label: "check-in lần 2", Action: "check-in", When: in[1]},
		{Label: "check-out lần 1", Action: "check-out", When: out[0]},
		{Label: "check-out lần 2", Action: "check-out", When: out[1]},
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

func nextJob(now time.Time, jobs []Job) *Job {
	for i := range jobs {
		if jobs[i].When.After(now) {
			j := jobs[i]
			return &j
		}
	}
	return nil
}

// markConsumed moves a job into the past so nextJob will not pick it again.
func markConsumed(jobs *[]Job, label string, now time.Time) {
	for i := range *jobs {
		if (*jobs)[i].Label == label {
			(*jobs)[i].When = now.Add(-time.Second)
			return
		}
	}
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

func runJob(runner Runner, job Job) bool {
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
		fmt.Fprintf(os.Stderr, "[%s] lần này bỏ qua (đã về Home + xóa đa nhiệm) — đợi job tiếp theo\n", job.Label)
		logFailure(job, err)
		return false
	}
	fmt.Printf("=== [%s] completed ===\n", job.Label)
	return true
}

func logFailure(job Job, err error) {
	_ = os.MkdirAll("logs", 0o755)
	path := filepath.Join("logs", time.Now().Format("2006-01-02")+".log")
	f, openErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if openErr != nil {
		fmt.Fprintf(os.Stderr, "log write failed: %v\n", openErr)
		return
	}
	defer f.Close()
	line := fmt.Sprintf("%s\t%s\t%s\t%v\n",
		time.Now().Format("2006-01-02 15:04:05"),
		job.Label,
		job.Action,
		err,
	)
	if _, werr := f.WriteString(line); werr != nil {
		fmt.Fprintf(os.Stderr, "log write failed: %v\n", werr)
	}
	fmt.Printf("logged failure → %s\n", path)
}

func timezone() string {
	if tz := os.Getenv("APPBIP_TZ"); tz != "" {
		return tz
	}
	return "Asia/Ho_Chi_Minh"
}

var _ Runner = (*people.Runner)(nil)
