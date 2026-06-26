package adb

import (
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type uiNode struct {
	Text        string `xml:"text,attr"`
	ContentDesc string `xml:"content-desc,attr"`
	Bounds      string `xml:"bounds,attr"`
	Clickable   string `xml:"clickable,attr"`
	Nodes       []uiNode `xml:"node"`
}

type uiHierarchy struct {
	Nodes []uiNode `xml:"node"`
}

var boundsRe = regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

func (c *Client) DumpUI(retries int) (string, error) {
	if retries <= 0 {
		retries = 5
	}
	var lastErr error
	for i := 0; i < retries; i++ {
		_, err := c.deviceCmd("shell", "uiautomator", "dump", "/sdcard/appbip_ui.xml")
		if err != nil {
			lastErr = err
			c.Sleep(2 * time.Second)
			continue
		}
		out, err := c.deviceCmd("shell", "cat", "/sdcard/appbip_ui.xml")
		if err != nil {
			lastErr = err
			c.Sleep(2 * time.Second)
			continue
		}
		if strings.Contains(out, "<hierarchy") {
			return out, nil
		}
		lastErr = fmt.Errorf("empty ui dump")
		c.Sleep(2 * time.Second)
	}
	return "", lastErr
}

func (c *Client) HasLabel(label string) (bool, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return false, err
	}
	_, _, found := findLabelCenter(dump, label)
	return found, nil
}

// HasAnyLabel returns true if any exact label is on screen.
func (c *Client) HasAnyLabel(labels ...string) (bool, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return false, err
	}
	for _, label := range labels {
		if _, _, ok := findLabelCenter(dump, label); ok {
			return true, nil
		}
	}
	return false, nil
}

// HasAnyLabelContains returns true if any label substring appears in text/content-desc.
func (c *Client) HasAnyLabelContains(substrs ...string) (bool, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return false, err
	}
	for _, substr := range substrs {
		if _, _, ok := findLabelContainsCenter(dump, substr); ok {
			return true, nil
		}
	}
	return false, nil
}

// WaitForAnyLabelContains polls until any substring appears or timeout.
func (c *Client) WaitForAnyLabelContains(timeout time.Duration, substrs ...string) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := c.HasAnyLabelContains(substrs...)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		c.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for labels containing %v", substrs)
}

// WaitUntilLabelGone polls until exact label is no longer on screen.
func (c *Client) WaitUntilLabelGone(label string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ok, err := c.HasLabel(label)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		c.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for %q to disappear", label)
}

// TapByLabelContains taps the best matching element (prefers clickable nodes).
func (c *Client) TapByLabelContains(substr string) (int, int, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return 0, 0, err
	}
	x, y, found := findLabelContainsCenterPreferClickable(dump, substr)
	if !found {
		return 0, 0, fmt.Errorf("label containing %q not found on screen", substr)
	}
	return x, y, c.Tap(x, y)
}

func (c *Client) TapByLabel(label string) (int, int, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return 0, 0, err
	}
	x, y, found := findLabelCenterPreferClickable(dump, label)
	if !found {
		return 0, 0, fmt.Errorf("label %q not found on screen", label)
	}
	return x, y, c.Tap(x, y)
}

// TapByLabelOrCoords tries label tap first, then falls back to fixed coordinates.
func (c *Client) TapByLabelOrCoords(label string, fallbackX, fallbackY int) (int, int, error) {
	x, y, err := c.TapByLabel(label)
	if err == nil {
		return x, y, nil
	}
	if err := c.Tap(fallbackX, fallbackY); err != nil {
		return 0, 0, err
	}
	return fallbackX, fallbackY, nil
}

// TapByLabelContainsOrCoords tries contains-match tap first, then fixed coordinates.
func (c *Client) TapByLabelContainsOrCoords(substr string, fallbackX, fallbackY int) (int, int, error) {
	x, y, err := c.TapByLabelContains(substr)
	if err == nil {
		return x, y, nil
	}
	if err := c.Tap(fallbackX, fallbackY); err != nil {
		return 0, 0, err
	}
	return fallbackX, fallbackY, nil
}

func findConfirmButtonCenter(dump string) (int, int, bool) {
	const minY = 850 // popup button zone on SM-X406B
	var matches []uiMatch
	decoder := xml.NewDecoder(strings.NewReader(dump))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, false
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "node" {
			continue
		}
		text, desc, bounds, clickable := readNodeAttrs(se)
		val := displayValue(text, desc)
		if !isConfirmButtonText(val) {
			continue
		}
		x, y, ok := boundsCenter(bounds)
		if !ok || y < minY {
			continue
		}
		matches = append(matches, uiMatch{
			x: x, y: y, area: boundsArea(bounds),
			clickable: clickable == "true",
			exact:     true,
		})
	}
	return pickBestMatch(matches)
}

func isConfirmButtonText(value string) bool {
	s := strings.ToLower(strings.TrimSpace(value))
	if s == "" || len(s) > 30 {
		return false
	}
	return s == "xác nhận" ||
		s == "xac nhan" ||
		s == "đồng ý" ||
		s == "dong y" ||
		s == "ok" ||
		strings.Contains(s, "xác nhận")
}

// WaitForConfirmPopup waits for a confirm button in the lower dialog area.
func (c *Client) WaitForConfirmPopup(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, _, ok, err := c.findConfirmButton(); err != nil {
			return err
		} else if ok {
			return nil
		}
		c.Sleep(2 * time.Second)
	}
	return fmt.Errorf("confirm popup not detected")
}

// FindConfirmButton locates the confirm button in the popup dialog.
func (c *Client) FindConfirmButton() (int, int, bool, error) {
	return c.findConfirmButton()
}

func (c *Client) findConfirmButton() (int, int, bool, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return 0, 0, false, err
	}
	x, y, ok := findConfirmButtonCenter(dump)
	return x, y, ok, nil
}

func findLabelCenter(dump, label string) (int, int, bool) {
	target := strings.TrimSpace(label)
	decoder := xml.NewDecoder(strings.NewReader(dump))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, false
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "node" {
			continue
		}
		var text, desc, bounds string
		for _, attr := range se.Attr {
			switch attr.Name.Local {
			case "text":
				text = strings.TrimSpace(attr.Value)
			case "content-desc":
				desc = strings.TrimSpace(attr.Value)
			case "bounds":
				bounds = attr.Value
			}
		}
		if text != target && desc != target {
			continue
		}
		x, y, ok := boundsCenter(bounds)
		if !ok {
			continue
		}
		return x, y, true
	}
	return 0, 0, false
}

func findLabelContainsCenter(dump, substr string) (int, int, bool) {
	target := strings.ToLower(strings.TrimSpace(substr))
	decoder := xml.NewDecoder(strings.NewReader(dump))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, false
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "node" {
			continue
		}
		var text, desc, bounds string
		for _, attr := range se.Attr {
			switch attr.Name.Local {
			case "text":
				text = strings.TrimSpace(attr.Value)
			case "content-desc":
				desc = strings.TrimSpace(attr.Value)
			case "bounds":
				bounds = attr.Value
			}
		}
		if !labelContains(text, target) && !labelContains(desc, target) {
			continue
		}
		x, y, ok := boundsCenter(bounds)
		if !ok {
			continue
		}
		return x, y, true
	}
	return 0, 0, false
}

func labelContains(value, target string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(value)), target)
}

type uiMatch struct {
	x, y, area int
	clickable  bool
	exact      bool
}

func findLabelCenterPreferClickable(dump, label string) (int, int, bool) {
	var matches []uiMatch
	decoder := xml.NewDecoder(strings.NewReader(dump))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, false
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "node" {
			continue
		}
		text, desc, bounds, clickable := readNodeAttrs(se)
		val := displayValue(text, desc)
		if !strings.EqualFold(val, label) {
			continue
		}
		x, y, ok := boundsCenter(bounds)
		if !ok {
			continue
		}
		matches = append(matches, uiMatch{
			x: x, y: y, area: boundsArea(bounds),
			clickable: clickable == "true",
			exact:     strings.EqualFold(val, label),
		})
	}
	return pickBestMatch(matches)
}

func findLabelContainsCenterPreferClickable(dump, substr string) (int, int, bool) {
	target := strings.ToLower(strings.TrimSpace(substr))
	var matches []uiMatch
	decoder := xml.NewDecoder(strings.NewReader(dump))
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, false
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "node" {
			continue
		}
		text, desc, bounds, clickable := readNodeAttrs(se)
		if !labelContains(text, target) && !labelContains(desc, target) {
			continue
		}
		x, y, ok := boundsCenter(bounds)
		if !ok {
			continue
		}
		val := displayValue(text, desc)
		matches = append(matches, uiMatch{
			x: x, y: y, area: boundsArea(bounds),
			clickable: clickable == "true",
			exact:     strings.EqualFold(val, substr) || strings.EqualFold(val, strings.ToUpper(substr)),
		})
	}
	return pickBestMatch(matches)
}

func readNodeAttrs(se xml.StartElement) (text, desc, bounds, clickable string) {
	for _, attr := range se.Attr {
		switch attr.Name.Local {
		case "text":
			text = strings.TrimSpace(attr.Value)
		case "content-desc":
			desc = strings.TrimSpace(attr.Value)
		case "bounds":
			bounds = attr.Value
		case "clickable":
			clickable = attr.Value
		}
	}
	return text, desc, bounds, clickable
}

func displayValue(text, desc string) string {
	if text != "" {
		return text
	}
	return desc
}

func pickBestMatch(matches []uiMatch) (int, int, bool) {
	if len(matches) == 0 {
		return 0, 0, false
	}
	best := matches[0]
	bestScore := matchScore(best)
	for _, m := range matches[1:] {
		if s := matchScore(m); s > bestScore {
			best = m
			bestScore = s
		}
	}
	return best.x, best.y, true
}

// Prefer clickable + exact label + smaller bounds (button-sized, not full-width container).
func matchScore(m uiMatch) int {
	score := 0
	if m.clickable {
		score += 100
	}
	if m.exact {
		score += 80
	}
	if m.area > 0 && m.area < 500000 {
		score += 20
	}
	score -= m.area / 5000
	return score
}

func boundsArea(bounds string) int {
	m := boundsRe.FindStringSubmatch(bounds)
	if len(m) != 5 {
		return 0
	}
	x1, _ := strconv.Atoi(m[1])
	y1, _ := strconv.Atoi(m[2])
	x2, _ := strconv.Atoi(m[3])
	y2, _ := strconv.Atoi(m[4])
	w := x2 - x1
	h := y2 - y1
	if w < 0 {
		w = -w
	}
	if h < 0 {
		h = -h
	}
	return w * h
}

func boundsCenter(bounds string) (int, int, bool) {
	m := boundsRe.FindStringSubmatch(bounds)
	if len(m) != 5 {
		return 0, 0, false
	}
	x1, _ := strconv.Atoi(m[1])
	y1, _ := strconv.Atoi(m[2])
	x2, _ := strconv.Atoi(m[3])
	y2, _ := strconv.Atoi(m[4])
	return (x1 + x2) / 2, (y1 + y2) / 2, true
}
