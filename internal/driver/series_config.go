package driver

import (
	"fmt"
	"strings"
	"time"

	"github.com/ralphite/agentrunner/internal/cron"
	"github.com/ralphite/agentrunner/internal/protocol"
)

// ApplySeriesConfigUpdate validates and resolves one editable-series patch.
// The returned spec is complete; cadenceChanged tells the runner whether to
// cancel its current timer and re-anchor from now.
func ApplySeriesConfigUpdate(current *DriverSpec, update *protocol.SeriesConfigControl) (*DriverSpec, bool, error) {
	if current == nil || update == nil {
		return nil, false, fmt.Errorf("series config update is required")
	}
	if current.schedule() != ScheduleInterval && current.schedule() != ScheduleCron {
		return nil, false, fmt.Errorf("only interval and cron series are editable")
	}
	if update.Prompt == nil && update.Schedule == nil && update.Interval == nil &&
		update.Cron == nil && update.Overlap == nil {
		return nil, false, fmt.Errorf("at least one editable field is required")
	}

	next := *current
	if update.Prompt != nil {
		prompt := strings.TrimSpace(*update.Prompt)
		if prompt == "" {
			return nil, false, fmt.Errorf("prompt must not be empty")
		}
		next.Prompt = prompt
	}
	if update.Overlap != nil {
		switch *update.Overlap {
		case OverlapSkip, OverlapCoalesce:
			next.Overlap = *update.Overlap
		default:
			return nil, false, fmt.Errorf("overlap must be skip or coalesce")
		}
	}

	if update.Schedule == nil && (update.Interval != nil || update.Cron != nil) {
		return nil, false, fmt.Errorf("schedule is required when cadence changes")
	}
	if update.Schedule != nil {
		switch *update.Schedule {
		case ScheduleInterval:
			if update.Interval == nil {
				return nil, false, fmt.Errorf("interval is required for schedule interval")
			}
			every, err := time.ParseDuration(strings.TrimSpace(*update.Interval))
			if err != nil || every < time.Second {
				return nil, false, fmt.Errorf("interval %q is not a duration of at least 1s", *update.Interval)
			}
			next.Schedule = ScheduleInterval
			next.Interval = strings.TrimSpace(*update.Interval)
			next.Cron = ""
		case ScheduleCron:
			if update.Cron == nil {
				return nil, false, fmt.Errorf("cron is required for schedule cron")
			}
			expr := strings.TrimSpace(*update.Cron)
			if _, err := cron.Parse(expr); err != nil {
				return nil, false, err
			}
			next.Schedule = ScheduleCron
			next.Cron = expr
			next.Interval = ""
		default:
			return nil, false, fmt.Errorf("schedule must be interval or cron")
		}
	}

	cadenceChanged := next.schedule() != current.schedule() ||
		next.Interval != current.Interval || next.Cron != current.Cron
	return &next, cadenceChanged, nil
}
