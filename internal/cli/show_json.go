package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Yeeef/pm/internal/pyjson"
)

// showObj is a JSON object built in Go, its keys in order.
type showObj [][2]any

// showDumps is json.dumps(v, indent=1): nil, bool, int, string, []any and showObj.
func showDumps(v any) string {
	var b strings.Builder
	showWrite(&b, v, 0)
	return b.String()
}

func showWrite(b *strings.Builder, v any, depth int) {
	pad := func(d int) string { return "\n" + strings.Repeat(" ", d) }
	switch t := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case int:
		b.WriteString(strconv.Itoa(t))
	case string:
		b.WriteString(pyjson.String(t, true))
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(pad(depth + 1))
			showWrite(b, e, depth+1)
		}
		b.WriteString(pad(depth) + "]")
	case showObj:
		if len(t) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, kv := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(pad(depth + 1))
			b.WriteString(pyjson.String(kv[0].(string), true) + ": ")
			showWrite(b, kv[1], depth+1)
		}
		b.WriteString(pad(depth) + "}")
	default:
		panic(fmt.Sprintf("showDumps: cannot write a %T", v))
	}
}
