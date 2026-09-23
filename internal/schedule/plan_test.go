package schedule

import (
	"testing"
	"time"
)

func TestWindowBounds(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 9, 16, 6, 0, 0, 0, loc) // Wednesday
	wins := dayWindows()
	if len(wins) != 2 {
		t.Fatalf("windows = %d", len(wins))
	}
	inStart, inEnd := windowBounds(day, wins[0])
	outStart, outEnd := windowBounds(day, wins[1])
	if inStart.Format("15:04:05") != "07:40:00" || inEnd.Format("15:04:05") != "08:25:00" {
		t.Fatalf("check-in window %s → %s", inStart.Format("15:04:05"), inEnd.Format("15:04:05"))
	}
	if outStart.Format("15:04:05") != "17:00:00" || outEnd.Format("15:04:05") != "17:40:00" {
		t.Fatalf("check-out window %s → %s", outStart.Format("15:04:05"), outEnd.Format("15:04:05"))
	}
}

func TestActiveWindow(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	day := time.Date(2026, 9, 16, 0, 0, 0, 0, loc)

	at := func(h, m, s int) time.Time {
		return time.Date(day.Year(), day.Month(), day.Day(), h, m, s, 0, loc)
	}

	w := activeWindow(at(7, 0, 0), nil)
	if w == nil || w.Action != "check-in" {
		t.Fatalf("before window want check-in, got %+v", w)
	}
	w = activeWindow(at(8, 24, 59), nil)
	if w == nil || w.Action != "check-in" {
		t.Fatalf("just before end want check-in, got %+v", w)
	}
	w = activeWindow(at(8, 25, 0), nil)
	if w == nil || w.Action != "check-out" {
		t.Fatalf("at 08:25 want check-out, got %+v", w)
	}
	w = activeWindow(at(8, 10, 0), map[string]bool{"check-in": true})
	if w == nil || w.Action != "check-out" {
		t.Fatalf("after success want check-out, got %+v", w)
	}
	w = activeWindow(at(18, 0, 0), nil)
	if w != nil {
		t.Fatalf("after both windows want nil, got %+v", w)
	}
	w = activeWindow(at(17, 10, 0), map[string]bool{"check-in": true, "check-out": true})
	if w != nil {
		t.Fatalf("both done want nil, got %+v", w)
	}
}

func TestNextWeekday_SkipsSatSun(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Ho_Chi_Minh")
	mon := time.Date(2026, 9, 14, 18, 0, 0, 0, loc)
	got := nextWeekday(mon)
	wantTue := time.Date(2026, 9, 15, 0, 0, 0, 0, loc)
	if !got.Equal(wantTue) {
		t.Fatalf("nextWeekday(Mon) = %s, want %s", got, wantTue)
	}
	fri := time.Date(2026, 9, 18, 18, 0, 0, 0, loc)
	got = nextWeekday(fri)
	wantMon := time.Date(2026, 9, 21, 0, 0, 0, 0, loc)
	if !got.Equal(wantMon) {
		t.Fatalf("nextWeekday(Fri) = %s, want %s", got, wantMon)
	}
}
