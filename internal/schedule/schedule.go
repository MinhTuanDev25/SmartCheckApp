package schedule

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/appbip/appbip/internal/people"

	_ "time/tzdata"
)

const (
	// Số lần thử mỗi action trong ngày; lần đầu OK là bỏ hết các lần sau.
	AttemptsPerAction = 15
	MinGap            = 90 * time.Second
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

	fmt.Printf("appbip schedule (Mon–Fri, %s) — %d check-in 07:45–08:08, %d check-out 17:00–17:30 (các lần sau bỏ nếu lần trước OK) — Ctrl+C to stop\n", loc.String(), AttemptsPerAction, AttemptsPerAction)

	var (
		planDate  time.Time
		jobs      []Job
		succeeded = map[string]bool{} // "check-in" / "check-out" đã OK trong ngày
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
		if runJob(runner, *next) {
			succeeded[next.Action] = true
		}
		markConsumed(&jobs, next.Label, time.Now().In(loc))
	}
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func planJobs(now time.Time) []Job {
	// Chỉ T2–T6; không chấm T7 / CN.
	if !isWorkday(now.Weekday()) {
		return nil
	}
	in := pickN(now, AttemptsPerAction, 7, 45, 0, 8, 8, 0, MinGap)
	out := pickN(now, AttemptsPerAction, 17, 0, 0, 17, 30, 0, MinGap)
	jobs := make([]Job, 0, 2*AttemptsPerAction)
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

// pickN picks n random times (hour:min:sec) in [h1:m1:s1, h2:m2:s2], ordered
// ascending, consecutive gaps at least minGap.
//
// Random offsets are drawn from the span left over after reserving (n-1)*minGap,
// then shifted by i*minGap. Rejection sampling would almost never succeed once n
// gets large relative to the window.
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

	gap := int(minGap.Seconds())
	free := span - (n-1)*gap
	if free < 0 {
		// Window too small for the requested gap — spread evenly instead.
		out := make([]time.Time, n)
		for i := 0; i < n; i++ {
			sec := 0
			if n > 1 {
				sec = span * i / (n - 1)
			}
			out[i] = start.Add(time.Duration(sec) * time.Second)
		}
		return out
	}

	offsets := make([]int, n)
	for i := range offsets {
		offsets[i] = rand.IntN(free + 1)
	}
	sortInts(offsets)

	out := make([]time.Time, n)
	for i, off := range offsets {
		out[i] = start.Add(time.Duration(off+i*gap) * time.Second)
	}
	return out
}

func sortInts(v []int) {
	for i := 1; i < len(v); i++ {
		j := i
		for j > 0 && v[j] < v[j-1] {
			v[j], v[j-1] = v[j-1], v[j]
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

func isWorkday(d time.Weekday) bool {
	return d >= time.Monday && d <= time.Friday
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
	for !isWorkday(d.Weekday()) {
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
