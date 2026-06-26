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

// TapByLabelContains taps the first element whose text/content-desc contains substr.
func (c *Client) TapByLabelContains(substr string) (int, int, error) {
	dump, err := c.DumpUI(5)
	if err != nil {
		return 0, 0, err
	}
	x, y, found := findLabelContainsCenter(dump, substr)
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
	x, y, found := findLabelCenter(dump, label)
	if !found {
		return 0, 0, fmt.Errorf("label %q not found on screen", label)
	}
	return x, y, c.Tap(x, y)
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
