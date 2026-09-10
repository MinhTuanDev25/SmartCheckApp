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

	fmt.Printf("appbip schedule (Mon–Fri, %s) — 4 check-in 07:50–08:07, 4 check-out 17:00–17:15 (các lần sau bỏ nếu lần trước OK) — Ctrl+C to stop\n", loc.String())

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

		// Các lần sau (cùng action) bỏ qua nếu đã thành công trong ngày.
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
	in := pickN(now, 4, 7, 50, 0, 8, 7, 0, 90*time.Second)
	out := pickN(now, 4, 17, 0, 0, 17, 15, 0, 90*time.Second)
	jobs := make([]Job, 0, 8)
	for i, t := range in {
		jobs = append(jobs, Job{
			Label:  fmt.Sprintf("check-in lần %d", i+1),
			Action: "check-in",
			When:   t,
		})
	}
	for i, t := range out {
		jobs = append(jobs, Job{
			Label:  fmt.Sprintf("check-out lần %d", i+1),
			Action: "check-out",
			When:   t,
		})
	}
	return jobs
}

// pickN picks n distinct random times (hour:min:sec) in [h1:m1:s1, h2:m2:s2],
// ordered ascending, consecutive gaps at least minGap.
func pickN(day time.Time, n, h1, m1, s1, h2, m2, s2 int, minGap time.Duration) []time.Time {
	if n <= 0 {
		return nil
	}
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), h1, m1, s1, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), h2, m2, s2, 0, loc)
	span := int(end.Sub(start).Seconds())
	if span < 0 {
		span = 0
	}
	for try := 0; try < 500; try++ {
		seen := map[int]struct{}{}
		times := make([]time.Time, 0, n)
		for len(times) < n {
			sec := 0
			if span > 0 {
				sec = rand.IntN(span + 1)
			}
			if _, ok := seen[sec]; ok {
				continue
			}
			seen[sec] = struct{}{}
			times = append(times, start.Add(time.Duration(sec)*time.Second))
		}
		sortTimes(times)
		ok := true
		for i := 1; i < len(times); i++ {
			if times[i].Sub(times[i-1]) < minGap {
				ok = false
				break
			}
		}
		if ok {
			return times
		}
	}
	// Fallback: evenly spaced from start by minGap (clamp to end).
	out := make([]time.Time, n)
	for i := 0; i < n; i++ {
		t := start.Add(time.Duration(i) * minGap)
		if t.After(end) {
			t = end
		}
		out[i] = t
	}
	return out
}

func sortTimes(times []time.Time) {
	for i := 1; i < len(times); i++ {
		j := i
		for j > 0 && times[j].Before(times[j-1]) {
			times[j], times[j-1] = times[j-1], times[j]
			j--
		}
	}
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
