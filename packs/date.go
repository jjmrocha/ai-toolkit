package packs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	currentDateToolName = "current_date"
	currentTimeToolName = "current_time"
	timeZoneToolName    = "time_zone"
	timeOfDayLayout     = "15:04:05.000"
)

var dateToolNames = []string{
	currentDateToolName,
	currentTimeToolName,
	timeZoneToolName,
}

var now = time.Now

type datePack struct {
	toolBox *tools.ToolBox
	once    sync.Once
}

func (p *datePack) Close() error {
	p.once.Do(func() {
		for _, name := range dateToolNames {
			p.toolBox.Remove(name)
		}
	})

	return nil
}

// DateTools registers the three tools that tell the model when it is in m:
// "current_date" returns the date as "2006-01-02", "current_time" the time of
// day as "15:04:05.000" on a 24-hour clock, and "time_zone" the host's zone
// with its offset from UTC. None of them takes an argument. The returned
// [ToolPack] removes the three again.
//
// A model has no clock of its own and its sense of the date comes from training
// data, so anything it dates without calling these is a guess. Register the pack
// wherever a date reaches the answer — a report header, a filing period, a
// valuation's as-of date.
//
// All three read the host clock through [time.Now], in the host's own zone.
// Nothing is launched to serve them, so [ToolPack.Close] only unregisters, and
// a dropped pack costs nothing beyond the tools staying registered.
//
// It fails with the error [tools.ToolBox.Add] returns when m rejects a
// registration, which leaves any tool already registered by the call in place.
func DateTools(m *tools.ToolBox) (ToolPack, error) {
	dateTool := llm.Tool{
		Name: currentDateToolName,
		Description: "Return today's date on the host, as \"2006-01-02\". Call it rather than assuming a " +
			"date: you have no clock, and the one you remember is the one you were trained on.",
		Schema: tools.NewObjectBuilder().Build(),
	}

	err := m.Add(dateTool, currentDate)
	if err != nil {
		return nil, err
	}

	timeTool := llm.Tool{
		Name: currentTimeToolName,
		Description: "Return the current time of day on the host, as \"15:04:05.000\" on a 24-hour clock. " +
			"It carries no date and no zone; call \"current_date\" and \"time_zone\" for those.",
		Schema: tools.NewObjectBuilder().Build(),
	}

	err = m.Add(timeTool, currentTime)
	if err != nil {
		return nil, err
	}

	zoneTool := llm.Tool{
		Name: timeZoneToolName,
		Description: "Return the host's time zone and its offset from UTC, as \"WEST (UTC+01:00)\". " +
			"The date and time the other two tools return are in this zone.",
		Schema: tools.NewObjectBuilder().Build(),
	}

	err = m.Add(zoneTool, timeZone)
	if err != nil {
		return nil, err
	}

	return &datePack{toolBox: m}, nil
}

func currentDate(context.Context, map[string]any) (string, error) {
	return now().Format(time.DateOnly), nil
}

func currentTime(context.Context, map[string]any) (string, error) {
	return now().Format(timeOfDayLayout), nil
}

func timeZone(context.Context, map[string]any) (string, error) {
	name, offset := now().Zone()

	return renderZone(name, offset), nil
}

func renderZone(name string, offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}

	utc := fmt.Sprintf("UTC%s%02d:%02d", sign, offset/3600, offset%3600/60)

	if name == "" {
		return utc
	}

	return fmt.Sprintf("%s (%s)", name, utc)
}
