package site

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/Yeeef/pm/internal/records"
)

// What the pm service adds to the pages: each is rendered when first asked for from one read of the records and
// items, its reply slots filled with forms and its status slot with the data's age.

// Served is one read of the records and the items, checked, whose pages render on demand.
type Served struct {
	recs  []*Record
	items *Items
	name  string
	dates Dates
}

// Serve checks the records and items as RenderPages does (every answered decision need cited) and returns the site
// they make; a check that fails is served instead of every page.
func Serve(recs []*Record, items *Items, siteName string, dates Dates, summaries map[string]*records.Summary) (*Served, error) {
	if err := CheckNeedsAnswered(recs, items, nil); err != nil {
		return nil, err
	}
	return &Served{WithDays(recs, items, summaries), items, siteName, dates}, nil
}

// Page is the page at path as RenderPages renders it; found false when the site has no such page. A record that fails
// to render fails only its own page here, where RenderPages fails the whole site.
func (s *Served) Page(path string) (string, bool, error) {
	if path == "index.html" {
		p, err := RenderIndex(s.recs, s.items, s.name, s.dates)
		return p, true, err
	}
	for _, r := range s.recs {
		if r.Out() == path {
			p, err := RenderRecord(r, s.recs, s.items, s.dates)
			return p, true, err
		}
	}
	return "", false, nil
}

// Reply is a site reply on its way to the work store, as its card shows it: State saving, sent or failed; Delivery
// for a sent one (a key of delivery); Error for a failed one.
type Reply struct{ State, Text, RID, Error, Delivery string }

// delivery is what a sent reply's card says of its push into the asking session, by the push's outcome.
var delivery = map[string]string{
	"delivered":          "delivered to the agent's session",
	"nothing to deliver": "the agent had read it already",
	"not running":        "not delivered: the session that asked is not running, the next one will see it",
	"failed":             "not delivered yet: the push failed; the pm service tries again every minute",
}

// The form a reply slot becomes. Each submit carries a reply id, new unless the text is the one the id was made for (a
// double click, or a failed reply sent again as it came back), so the pm service stores a resent reply once.
const replyForm = `<form class="reply" method="post" action="/reply" data-text="%[4]s" onsubmit="` +
	`const t = this.elements.text.value, r = this.elements.rid; ` +
	`if (!r.value || this.dataset.text !== t) { r.value = crypto.randomUUID(); this.dataset.text = t; }">` +
	`<input type="hidden" name="token" value="%[1]s"><input type="hidden" name="id" value="%[2]s">` +
	`<input type="hidden" name="rid" value="%[5]s"><textarea name="text" rows="3" required ` +
	`placeholder="%[3]s">%[4]s</textarea><button type="submit">Send reply</button></form>`

var replyHints = map[string]string{"decision": "Your answer: the option you choose, or what you want instead",
	"action": "The evidence that it is done: a command's output, a link"}

var slotRE = regexp.MustCompile(`<!--pm-reply (\S+) (decision|action)-->`)

// pyEscape is Python's html.escape: quotes too, the single one as &#x27;.
var pyEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#x27;").Replace

// FillReplies is page with each card's reply slot replaced by its form, carrying token for the POST. replies holds
// the replies still on their way to the work store, by need id: each shows with its text above its card's form, and a
// failed one puts its text back in the box with its reply id, so sending it again unchanged stores it once.
func FillReplies(page, token string, replies map[string]Reply) string {
	return slotRE.ReplaceAllStringFunc(page, func(m string) string {
		g := slotRE.FindStringSubmatch(m)
		r, ok := replies[html.UnescapeString(g[1])]
		note := ""
		if ok {
			said, known := delivery[r.Delivery]
			if !known {
				said = NotDelivered
			}
			text := mustRender(commentMD, r.Text)
			switch r.State {
			case "saving":
				note = `<div class="reply-item"><p class="k replied">Saving your reply…</p>` + text + `</div>`
			case "sent":
				note = `<div class="reply-item"><p class="k replied">Reply saved; ` + said + `.</p>` + text + `</div>`
			case "failed":
				note = `<p class="k failed">Your reply was not stored: ` + pyEscape(r.Error) + ` Its text is back in ` +
					`the box; send it again.</p>`
			}
		}
		text, rid := "", ""
		if ok && r.State == "failed" {
			text, rid = pyEscape(r.Text), pyEscape(r.RID)
		}
		return note + fmt.Sprintf(replyForm, pyEscape(token), g[1], replyHints[g[2]], text, rid)
	})
}

// ServeBehind is how many seconds a served page may be behind the records and the work store before it says so.
const ServeBehind = 10

// statusScript keeps the stated age current and polls /version: an unchanged page takes the newer age; a changed one
// never reloads by itself (owner decision: auto reloads flash the page) but shows a sticky "Newer data: reload" banner
// the owner clicks. That reload keeps the reader where they were: it saves the scroll position and drops the URL's
// #fragment (a reply's redirect to its card), which would otherwise re-jump on every reload. It asks first when a
// reply box holds unsent text, which a reload would discard.
var statusScript = `(() => {
const el = document.currentScript.previousElementSibling, behind = ` + fmt.Sprint(ServeBehind) + `, key = "pm-scroll:" + location.pathname;
let newer = false, gone = false;
function reload() {
  const draft = [...document.querySelectorAll("textarea")].some(t => t.value.trim());
  if (draft && !confirm("Reload and discard the reply you have not sent?")) return;
  try { sessionStorage.setItem(key, String(scrollY)); } catch (e) {}
  if (location.hash) history.replaceState(null, "", location.pathname + location.search);
  location.reload();
}
let saved = null;
try { saved = sessionStorage.getItem(key); sessionStorage.removeItem(key); } catch (e) {}
if (saved !== null) addEventListener("load", () => scrollTo(0, +saved));
function show() {
  const asof = +el.dataset.asof, age = Math.max(0, Math.round(Date.now() / 1000 - asof));
  const at = new Date(asof * 1000).toLocaleTimeString([], {hour12: false});
  el.classList.toggle("behind", !newer && age > behind);
  el.textContent = ` + "`Data as of ${at}` + (newer ? \"; newer data exists, reload to see it\" : ` (${age} s ago)`)" + ` +
    (gone ? "; the server is not answering" : !newer && age > behind ? "; other sessions are writing, so it is behind" : "");
  if (newer && !document.querySelector(".newer")) {
    document.body.insertAdjacentHTML("afterbegin", '<a class="newer" href="">Newer data: reload</a>');
    document.querySelector(".newer").onclick = e => { e.preventDefault(); reload(); };
  }
}
async function poll() {
  try {
    const r = await fetch("/version?page=" + encodeURIComponent(location.pathname), {cache: "no-store"});
    const v = await r.json();
    gone = false;
    if (v.page === el.dataset.page) el.dataset.asof = v.asof;
    else newer = true;
  } catch (e) { gone = true; }
  show();
  setTimeout(poll, 2000);
}
show(); setInterval(show, 1000); setTimeout(poll, 2000);
})();`

// FillStatus is page with its status slot replaced by the line stating its data's age: as of asOf, the time from
// which the records and items it shows are known current, and digest, which /version compares.
func FillStatus(page string, asOf time.Time, digest string, now time.Time) string {
	age := max(0, int(math.RoundToEven(now.Sub(asOf).Seconds())))
	text := fmt.Sprintf("Data as of %s (%d s ago)", asOf.Local().Format("15:04:05"), age)
	behind := ""
	if age > ServeBehind {
		text += "; other sessions are writing, so it is behind"
		behind = " behind"
	}
	line := fmt.Sprintf(`<p class="asof%s" data-asof="%.3f" data-page="%s">%s</p>`+"\n<script>%s</script>", behind,
		float64(asOf.UnixMicro())/1e6, digest, text, statusScript)
	return strings.Replace(page, StatusSlot, line, 1)
}
