package schedule

import (
	"strings"
	"testing"
	"time"
)

func TestPlanJobs_AttemptsAndWindows(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	// Wednesday.
	day := time.Date(2026, 9, 16, 6, 0, 0, 0, loc)

	for run := 0; run < 200; run++ {
		jobs := planJobs(day)
		if len(jobs) != 2*AttemptsPerAction {
			t.Fatalf("got %d jobs, want %d", len(jobs), 2*AttemptsPerAction)
		}

		var in, out []time.Time
		for _, j := range jobs {
			switch j.Action {
			case "check-in":
				in = append(in, j.When)
			case "check-out":
				out = append(out, j.When)
			default:
				t.Fatalf("unexpected action %q", j.Action)
			}
		}

		assertWindow(t, in, day, loc, 7, 50, 0, 8, 7, 0)
		assertWindow(t, out, day, loc, 17, 0, 0, 17, 15, 0)
	}
}

func assertWindow(t *testing.T, times []time.Time, day time.Time, loc *time.Location, h1, m1, s1, h2, m2, s2 int) {
	t.Helper()
	if len(times) != AttemptsPerAction {
		t.Fatalf("got %d times, want %d", len(times), AttemptsPerAction)
	}
	start := time.Date(day.Year(), day.Month(), day.Day(), h1, m1, s1, 0, loc)
	end := time.Date(day.Year(), day.Month(), day.Day(), h2, m2, s2, 0, loc)
	for i, ts := range times {
		if ts.Before(start) || ts.After(end) {
			t.Fatalf("time %s out of window [%s, %s]", ts.Format("15:04:05"), start.Format("15:04:05"), end.Format("15:04:05"))
		}
		if i > 0 && ts.Sub(times[i-1]) < MinGap {
			t.Fatalf("gap %s < %s between %s and %s", ts.Sub(times[i-1]), MinGap,
				times[i-1].Format("15:04:05"), ts.Format("15:04:05"))
		}
	}
}

func TestPlanJobs_WeekendEmpty(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	sat := time.Date(2026, 9, 19, 8, 0, 0, 0, loc)
	if jobs := planJobs(sat); len(jobs) != 0 {
		t.Fatalf("weekend should have no jobs, got %d", len(jobs))
	}
}

func TestPlanJobs_LabelsNumbered(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	jobs := planJobs(time.Date(2026, 9, 16, 6, 0, 0, 0, loc))
	last := jobs[AttemptsPerAction-1].Label
	if !strings.HasSuffix(last, "lần 10") {
		t.Fatalf("last check-in label = %q, want suffix \"lần 10\"", last)
	}
}
