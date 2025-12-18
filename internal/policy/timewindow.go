package policy

import (
	"errors"
	"strings"
	"time"
)

// InTimeSpan checks if now is within the provided timespan definition.
// Supported syntax is identical to rules timespans: HH:MM-HH:MM, optional weekdays, optional |tz=Area/City.
func InTimeSpan(def string, now time.Time) bool {
	ts, err := parsePolicyTimeSpan(def, now.Location())
	if err != nil {
		return false
	}
	current := now.In(ts.loc)
	if len(ts.weekdays) > 0 {
		if !ts.weekdays[current.Weekday()] {
			return false
		}
	}
	cur := time.Date(0, 1, 1, current.Hour(), current.Minute(), 0, 0, ts.loc)
	start := time.Date(0, 1, 1, ts.start.Hour(), ts.start.Minute(), 0, 0, ts.loc)
	end := time.Date(0, 1, 1, ts.end.Hour(), ts.end.Minute(), 0, 0, ts.loc)

	if ts.crossMidnight {
		if cur.After(start) || cur.Equal(start) {
			return true
		}
		if cur.Before(end) || cur.Equal(end) {
			return true
		}
		return false
	}
	return (cur.After(start) || cur.Equal(start)) && cur.Before(end)
}

type span struct {
	start         time.Time
	end           time.Time
	crossMidnight bool
	weekdays      map[time.Weekday]bool
	loc           *time.Location
}

func parsePolicyTimeSpan(def string, defaultLoc *time.Location) (*span, error) {
	if defaultLoc == nil {
		defaultLoc = time.Local
	}
	parts := strings.Split(def, "|")
	timePart := strings.TrimSpace(parts[0])
	if timePart == "" {
		return nil, errors.New("empty timespan")
	}
	weekdayPart := ""
	if strings.Contains(timePart, "@") {
		s := strings.SplitN(timePart, "@", 2)
		weekdayPart = s[0]
		timePart = s[1]
	}
	rangeParts := strings.Split(timePart, "-")
	if len(rangeParts) != 2 {
		return nil, errors.New("invalid range")
	}
	start, err := time.Parse("15:04", rangeParts[0])
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("15:04", rangeParts[1])
	if err != nil {
		return nil, err
	}
	loc := defaultLoc
	if len(parts) > 1 {
		for _, opt := range parts[1:] {
			opt = strings.TrimSpace(opt)
			if strings.HasPrefix(opt, "tz=") {
				if l, err := time.LoadLocation(strings.TrimPrefix(opt, "tz=")); err == nil {
					loc = l
				}
			}
		}
	}
	weekdays, err := parseWeekdays(weekdayPart)
	if err != nil {
		return nil, err
	}
	return &span{
		start:         start,
		end:           end,
		crossMidnight: start.After(end),
		weekdays:      weekdays,
		loc:           loc,
	}, nil
}

func parseWeekdays(def string) (map[time.Weekday]bool, error) {
	result := make(map[time.Weekday]bool)
	def = strings.TrimSpace(def)
	if def == "" {
		return result, nil
	}
	parts := strings.Split(def, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "-") {
			r := strings.SplitN(p, "-", 2)
			if len(r) != 2 {
				return nil, errors.New("invalid weekday range")
			}
			start, err := parseWeekday(r[0])
			if err != nil {
				return nil, err
			}
			end, err := parseWeekday(r[1])
			if err != nil {
				return nil, err
			}
			for i := start; ; i = (i + 1) % 7 {
				result[i] = true
				if i == end {
					break
				}
			}
		} else {
			w, err := parseWeekday(p)
			if err != nil {
				return nil, err
			}
			result[w] = true
		}
	}
	return result, nil
}

func parseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(s) {
	case "mon", "monday":
		return time.Monday, nil
	case "tue", "tues", "tuesday":
		return time.Tuesday, nil
	case "wed", "wednesday":
		return time.Wednesday, nil
	case "thu", "thursday", "thurs":
		return time.Thursday, nil
	case "fri", "friday":
		return time.Friday, nil
	case "sat", "saturday":
		return time.Saturday, nil
	case "sun", "sunday":
		return time.Sunday, nil
	}
	return time.Sunday, errors.New("invalid weekday")
}
