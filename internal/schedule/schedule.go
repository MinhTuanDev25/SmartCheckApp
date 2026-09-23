package schedule

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/appbip/appbip/internal/people"

	_ "time/tzdata"
)

const retryDelay = 10 * time.Second

// actionWindow là khung retry liên tục. Hết giờ hoặc đã thành công thì dừng.
type actionWindow struct {
	Action string
	Label  string
	StartH int
	StartM int
	StartS int
	EndH   int
	EndM   int
	EndS   int
}

func dayWindows() []actionWindow {
	return []actionWindow{
		{Action: "check-in", Label: "check-in", StartH: 7, StartM: 40, StartS: 0, EndH: 8, EndM: 25, EndS: 0},
		{Action: "check-out", Label: "check-out", StartH: 17, StartM: 0, StartS: 0, EndH: 17, EndM: 40, EndS: 0},
	}
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

	fmt.Printf("appbip schedule (Mon–Fri, %s) — check-in 07:40–08:25, check-out 17:00–17:40, retry mỗi %s đến khi OK hoặc hết giờ — Ctrl+C to stop\n", loc.String(), retryDelay)

	var (
		planDate  time.Time
		succeeded = map[string]bool{}
		attempt   = map[string]int{}
	)

	for {
		now := time.Now().In(loc)
		today := dateOnly(now)
		if !planDate.Equal(today) {
			planDate = today
			succeeded = map[string]bool{}
			attempt = map[string]int{}
			printPlan(now)
		}

		if !isWorkday(now.Weekday()) {
			wake := nextWeekday(now)
			fmt.Printf("off day — sleeping until %s\n", wake.Format("2006-01-02 15:04:05"))
			if err := sleepUntil(wake); err != nil {
				return err
			}
			continue
		}

		win := activeWindow(now, succeeded)
		if win == nil {
			wake := nextWeekday(now)
			fmt.Printf("no remaining windows today — sleeping until %s\n", wake.Format("2006-01-02 15:04:05"))
			if err := sleepUntil(wake); err != nil {
				return err
			}
			continue
		}

		start, end := windowBounds(now, *win)
		if now.Before(start) {
			fmt.Printf("next: %s at %s (retry đến %s)\n", win.Label, start.Format("15:04:05"), end.Format("15:04:05"))
			if err := sleepUntil(start); err != nil {
				return err
			}
			continue
		}

		attempt[win.Action]++
		n := attempt[win.Action]
		label := fmt.Sprintf("%s lần %d", win.Label, n)
		if runAttempt(runner, label, win.Action) {
			succeeded[win.Action] = true
			fmt.Printf("%s thành công — không retry nữa\n", win.Label)
			continue
		}

		now = time.Now().In(loc)
		_, end = windowBounds(now, *win)
		next := now.Add(retryDelay)
		if !next.Before(end) {
			fmt.Printf("%s hết khung %s — dừng retry\n", win.Label, end.Format("15:04:05"))
			succeeded[win.Action] = true // đóng cửa sổ, không thử thêm trong ngày
			continue
		}
		fmt.Printf("%s fail — cleanup xong, retry sau %s\n", label, retryDelay)
		if err := sleepUntil(next); err != nil {
			return err
		}
	}
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func windowBounds(day time.Time, w actionWindow) (time.Time, time.Time) {
	loc := day.Location()
	start := time.Date(day.Year(), day.Month(), day.Day(), w.StartH, w.StartM, w.StartS, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), w.EndH, w.EndM, w.EndS, 0, loc)
	return start, end
}

// activeWindow trả cửa sổ đầu tiên chưa thành công và chưa qua giờ kết thúc.
func activeWindow(now time.Time, succeeded map[string]bool) *actionWindow {
	for _, w := range dayWindows() {
		if succeeded[w.Action] {
			continue
		}
		_, end := windowBounds(now, w)
		if !now.Before(end) {
			continue
		}
		cp := w
		return &cp
	}
	return nil
}

func isWorkday(d time.Weekday) bool {
	return d >= time.Monday && d <= time.Friday
}

func printPlan(now time.Time) {
	if !isWorkday(now.Weekday()) {
		fmt.Printf("=== %s (%s) weekend — no jobs ===\n", now.Format("2006-01-02"), now.Weekday())
		return
	}
	fmt.Printf("=== %s (%s) ===\n", now.Format("2006-01-02"), now.Weekday())
	for _, w := range dayWindows() {
		start, end := windowBounds(now, w)
		fmt.Printf("  %-12s %s → %s  retry mỗi %s đến khi OK\n", w.Label, start.Format("15:04:05"), end.Format("15:04:05"), retryDelay)
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

func runAttempt(runner Runner, label, action string) bool {
	now := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("\n=== [%s] %s ===\n", now, label)
	var err error
	switch action {
	case "check-in":
		err = runner.CheckIn()
	case "check-out":
		err = runner.CheckOut()
	default:
		err = fmt.Errorf("unknown action %q", action)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] fail (đã về Home + xóa đa nhiệm)\n", label)
		logFailure(label, action, err)
		return false
	}
	fmt.Printf("=== [%s] completed ===\n", label)
	return true
}

func logFailure(label, action string, err error) {
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
		label,
		action,
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
