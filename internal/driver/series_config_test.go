package driver_test

import (
	"strings"
	"testing"

	"github.com/ralphite/agentrunner/internal/driver"
	"github.com/ralphite/agentrunner/internal/protocol"
)

func strptr(value string) *string { return &value }

func TestApplySeriesConfigUpdate(t *testing.T) {
	current := &driver.DriverSpec{
		Name: "daily", Prompt: "Old prompt", Schedule: driver.ScheduleInterval,
		Interval: "30m", Overlap: driver.OverlapSkip,
	}
	next, cadenceChanged, err := driver.ApplySeriesConfigUpdate(current, &protocol.SeriesConfigControl{
		Prompt: strptr(" New prompt "),
	})
	if err != nil || cadenceChanged || next.Prompt != "New prompt" || next.Interval != "30m" {
		t.Fatalf("prompt update = %+v changed=%v err=%v", next, cadenceChanged, err)
	}
	next, cadenceChanged, err = driver.ApplySeriesConfigUpdate(current, &protocol.SeriesConfigControl{
		Schedule: strptr(driver.ScheduleCron), Cron: strptr("0 9 * * 1-5"),
		Overlap: strptr(driver.OverlapCoalesce),
	})
	if err != nil || !cadenceChanged || next.Schedule != driver.ScheduleCron ||
		next.Cron != "0 9 * * 1-5" || next.Interval != "" ||
		next.Overlap != driver.OverlapCoalesce {
		t.Fatalf("cadence update = %+v changed=%v err=%v", next, cadenceChanged, err)
	}
	for _, tc := range []protocol.SeriesConfigControl{
		{},
		{Prompt: strptr(" ")},
		{Schedule: strptr(driver.ScheduleInterval), Interval: strptr("fast")},
		{Schedule: strptr(driver.ScheduleCron), Cron: strptr("0 9 * *")},
		{Overlap: strptr("interrupt")},
	} {
		if _, _, err := driver.ApplySeriesConfigUpdate(current, &tc); err == nil {
			t.Fatalf("invalid update accepted: %+v", tc)
		}
	}
	selfPaced := *current
	selfPaced.Schedule = driver.ScheduleSelfPaced
	if _, _, err := driver.ApplySeriesConfigUpdate(&selfPaced, &protocol.SeriesConfigControl{
		Prompt: strptr("x"),
	}); err == nil || !strings.Contains(err.Error(), "only interval and cron") {
		t.Fatalf("self-paced edit error = %v", err)
	}
}
