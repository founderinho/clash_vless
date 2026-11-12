// Package loghub defines the structured activity records shared by every UI.
package loghub

import (
	"fmt"
	"strings"
	"time"
)

type Level string

const (
	Debug   Level = "debug"
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
)

var Levels = []Level{Info, Warning, Error, Debug}

type Record struct {
	Time    time.Time `json:"time"`
	Level   Level     `json:"level"`
	Module  string    `json:"module"`
	Element string    `json:"element,omitempty"`
	Message string    `json:"message"`
}

func New(level Level, module, element, format string, args ...any) Record {
	if module == "" {
		module = "app"
	}
	return Record{Time: time.Now(), Level: level, Module: module, Element: element, Message: fmt.Sprintf(format, args...)}
}

func (r Record) Origin() string {
	if r.Element == "" {
		return r.Module
	}
	return r.Module + "/" + r.Element
}

func (r Record) LevelLabel() string {
	switch r.Level {
	case Warning:
		return "WARN"
	case Error:
		return "ERROR"
	case Debug:
		return "DEBUG"
	default:
		return "INFO"
	}
}

func (r Record) String() string {
	when := r.Time
	if when.IsZero() {
		when = time.Now()
	}
	return fmt.Sprintf("%s %-5s %s %s", when.Local().Format("15:04:05.000"), r.LevelLabel(), r.Origin(), r.Message)
}

// ParseLine keeps clients compatible with pre-structured daemons.
func ParseLine(line string) Record {
	// The level is padded to 5 columns, so the gap before the origin varies.
	stamp, rest, _ := strings.Cut(line, " ")
	label, rest, _ := strings.Cut(rest, " ")
	origin, message, ok := strings.Cut(strings.TrimLeft(rest, " "), " ")
	if at, err := time.ParseInLocation("15:04:05.000", stamp, time.Local); err == nil && ok {
		level := Info
		switch label {
		case "WARN":
			level = Warning
		case "ERROR":
			level = Error
		case "DEBUG":
			level = Debug
		}
		module, element, _ := strings.Cut(origin, "/")
		return Record{Time: at, Level: level, Module: module, Element: element, Message: message}
	}
	return New(Info, "engine", "", "%s", line)
}

type Filter struct {
	Levels map[Level]bool
	Module string
	Query  string
}

func DefaultFilter() Filter {
	return Filter{Levels: map[Level]bool{Info: true, Warning: true, Error: true}}
}

func (f Filter) Shows(level Level) bool {
	return f.Levels == nil || f.Levels[level]
}

func (f Filter) Allows(record Record) bool {
	if !f.Shows(record.Level) || f.Module != "" && f.Module != record.Module {
		return false
	}
	if strings.TrimSpace(f.Query) == "" {
		return true // formatting every record just to match nothing is the costly part
	}
	return TextMatches(record.String(), f.Query)
}

// TextMatches uses " | " for alternatives and " & " for required terms.
func TextMatches(text, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	text = strings.ToLower(text)
	for _, alternative := range strings.Split(query, " | ") {
		matches := true
		terms := 0
		for _, term := range strings.Split(alternative, " & ") {
			term = strings.ToLower(strings.TrimSpace(term))
			if term == "" {
				continue
			}
			terms++
			if !strings.Contains(text, term) {
				matches = false
				break
			}
		}
		if matches && terms > 0 {
			return true
		}
	}
	return false
}
