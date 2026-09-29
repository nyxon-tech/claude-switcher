package cli

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/nyxon-tech/claude-switcher/v3/internal/i18n"
	"github.com/nyxon-tech/claude-switcher/v3/internal/transcript"
)

func (a *app) usageCommand() *cobra.Command {
	var days int
	cmd := a.command("usage", "usage", "more", nargs(0, 0), func(cmd *cobra.Command, _ []string) error {
		return a.usage(cmd.Context(), max(days, 1))
	})
	cmd.Flags().IntVar(&days, "days", 30, a.txt(i18n.T("cli.flag.days")))
	return cmd
}

func (a *app) usage(ctx context.Context, days int) error {
	env, err := a.open()
	if err != nil {
		return err
	}
	var done, total atomic.Int64
	var stats transcript.Stats
	err = a.spin(ctx, func() string {
		if t := total.Load(); t > 0 {
			return i18n.T("cli.usage.reading", "n", t, "pct", i18n.Digits(fmt.Sprintf("%d%%", done.Load()*100/t)))
		}
		return i18n.T("state.loading")
	}, func(ctx context.Context) error {
		// Reading every transcript can take a while the first time; ctrl+c stops waiting for it.
		read := make(chan error, 1)
		go func() {
			var err error
			stats, err = env.Usage(func(d, t int) { done.Store(int64(d)); total.Store(int64(t)) })
			read <- err
		}()
		select {
		case err := <-read:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	if err != nil {
		return err
	}
	var sum transcript.Usage
	for _, u := range stats.ByModel {
		sum = sum.Add(u)
	}
	if a.flags.json {
		return a.json(struct {
			transcript.Stats
			Total       transcript.Usage `json:"total"`
			TotalTokens int64            `json:"totalTokens"`
		}{stats, sum, sum.Total()})
	}
	if sum.Total() == 0 {
		a.note(a.out, i18n.T("cli.usage.none", "path", env.Projects))
		return nil
	}
	a.pairs([][2]string{
		{i18n.T("cli.usage.total"), a.st.bold.Render(a.txt(i18n.Compact(sum.Total())))},
		{i18n.T("cli.usage.output"), a.txt(i18n.Compact(sum.Output))},
		{i18n.T("cli.usage.sessions"), a.txt(i18n.N(int64(stats.Sessions)))},
		{i18n.T("cli.usage.span"), a.txt(i18n.Date(stats.First.Local()) + " – " + i18n.Date(stats.Last.Local()))},
	})

	a.heading(i18n.T("cli.usage.by-model"))
	a.print(a.out, a.shares(i18n.T("cli.usage.model"), stats.ByModel, sum.Total(), a.fit))
	a.heading(i18n.T("cli.usage.days", "n", days))
	a.print(a.out, a.daily(stats.ByDay, days))
	a.heading(i18n.T("cli.usage.projects"))
	a.print(a.out, a.shares(i18n.T("cli.usage.project"), top(stats.ByProject, 8), sum.Total(), func(cwd string, width int) string {
		return a.fit(filepath.Base(cwd), width)
	}))
	fmt.Fprintln(a.out)
	a.note(a.out, i18n.T("cli.usage.note"))
	return nil
}

// top keeps the n biggest entries.
func top(m map[string]transcript.Usage, n int) map[string]transcript.Usage {
	keys := slices.SortedFunc(maps.Keys(m), func(x, y string) int { return cmp.Compare(m[y].Total(), m[x].Total()) })
	out := map[string]transcript.Usage{}
	for _, k := range keys[:min(n, len(keys))] {
		out[k] = m[k]
	}
	return out
}

// shares is a table of token counts, biggest first, each with its share of all and a bar.
func (a *app) shares(header string, m map[string]transcript.Usage, all int64, name func(key string, width int) string) string {
	keys := slices.SortedFunc(maps.Keys(m), func(x, y string) int {
		return cmp.Or(cmp.Compare(m[y].Total(), m[x].Total()), strings.Compare(x, y))
	})
	bar := min(20, max(8, (a.width-40)/2))
	rows := make([][]string, len(keys))
	for i, k := range keys {
		share := float64(m[k].Total()) / float64(max(all, 1))
		rows[i] = []string{"", a.txt(i18n.Compact(m[k].Total())), a.txt(i18n.Digits(fmt.Sprintf("%.1f%%", share*100))), a.bar(share, bar)}
	}
	headers := []string{header, i18n.T("cli.usage.tokens"), i18n.T("cli.usage.share"), ""}
	a.fitColumn(headers, rows, 0, func(i, width int) string { return name(keys[i], width) })
	return a.table(headers, rows, 1, 2)
}

// bar fills width cells by share, from the reading-start edge.
func (a *app) bar(share float64, width int) string {
	n := int(share*float64(width) + 0.5)
	full, empty := a.st.accent.Render(strings.Repeat("█", n)), a.st.border.Render(strings.Repeat("░", width-n))
	if a.mirror {
		return empty + full
	}
	return full + empty
}

// daily is a column chart of output tokens over the last days, oldest at the reading start,
// with the first and last dates under it. A window narrower than the days gets a column for
// every few days.
func (a *app) daily(byDay map[string]transcript.Usage, days int) string {
	levels := []rune("▁▂▃▄▅▆▇█")
	today := time.Now()
	values := make([]int64, min(days, a.width-1))
	for d := range days {
		values[d*len(values)/days] += byDay[today.AddDate(0, 0, d-days+1).Format(time.DateOnly)].Output
	}
	peak := slices.Max(values)
	cells := make([]string, len(values))
	for i, v := range values {
		cells[i] = a.st.border.Render("▁")
		if v > 0 {
			cells[i] = a.st.accent.Render(string(levels[v*int64(len(levels)-1)/peak]))
		}
	}
	first, last := a.txt(i18n.Date(today.AddDate(0, 0, 1-days))), a.txt(i18n.Date(today))
	if a.mirror {
		slices.Reverse(cells)
		first, last = last, first
	}
	chart := strings.Join(cells, "")
	gap := len(cells) - lipgloss.Width(first) - lipgloss.Width(last)
	if gap < 1 {
		return chart
	}
	return chart + "\n" + a.st.muted.Render(first+strings.Repeat(" ", gap)+last)
}
