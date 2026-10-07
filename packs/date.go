package packs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	currentDateToolName = "current_date"
	currentTimeToolName = "current_time"
	timeZoneToolName    = "time_zone"
	timeOfDayLayout     = "15:04:05.000"
)

var now = time.Now

const dateInstruction = `You have no clock of your own: the date and time you
remember are the ones you were trained on. Call these tools rather than assuming
when it is, wherever the answer you are writing carries a date, a time or a
deadline — and call them again rather than reusing an earlier answer across a
long session, because the day may have moved.`

type datePack struct {
	toolBox *tools.ToolBox
	names   []string
	once    sync.Once
}

func (p *datePack) Close() error {
	p.once.Do(func() {
		for _, name := range p.names {
			p.toolBox.Remove(name)
		}
	})

	return nil
}

func (p *datePack) Instructions(_ context.Context) *mcp.Instruction {
	return &mcp.Instruction{
		Name: "date",
		Text: dateInstruction,
	}
}

// DateTools registers three tools in m that tell the model when it is:
// "current_date" returns the date as "2006-01-02", "current_time" the time of
// day as "15:04:05.000" on a 24-hour clock, and "time_zone" the host's zone and
// its offset from UTC. None takes an argument, and all read [time.Now] in the
// host's zone.
//
// A model has no clock; a date it writes without these tools comes from its
// training data. Register the pack wherever a date reaches the answer.
//
// Nothing is launched, so [ToolPack.Close] only unregisters. If m rejects a
// registration, DateTools returns the error from [tools.ToolBox.Add] and leaves
// the tools it already registered in place.
func DateTools(m *tools.ToolBox) (ToolPack, error) {
	dateTool := llm.Tool{
		Name: currentDateToolName,
		Description: "Return today's date on the host, as \"2006-01-02\". Call it rather than assuming a " +
			"date: you have no clock, and the one you remember is the one you were trained on.",
		Schema: tools.NewObjectBuilder().Build(),
	}

	timeTool := llm.Tool{
		Name: currentTimeToolName,
		Description: "Return the current time of day on the host, as \"15:04:05.000\" on a 24-hour clock. " +
			"It carries no date and no zone; call \"current_date\" and \"time_zone\" for those.",
		Schema: tools.NewObjectBuilder().Build(),
	}

	zoneTool := llm.Tool{
		Name: timeZoneToolName,
		Description: "Return the host's time zone and its offset from UTC, as \"WEST (UTC+01:00)\". " +
			"The date and time the other two tools return are in this zone.",
		Schema: tools.NewObjectBuilder().Build(),
	}

	names, err := register(m, []registration{
		{tool: dateTool, handler: currentDate},
		{tool: timeTool, handler: currentTime},
		{tool: zoneTool, handler: timeZone},
	})
	if err != nil {
		return nil, err
	}

	return &datePack{toolBox: m, names: names}, nil
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
