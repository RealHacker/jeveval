package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
)

type repoRow struct {
	ID                                                         int64
	Name, Description, URL, Language, Starred, Pushed, License string
	Stars, Activity                                            int
	LOC                                                        sql.NullInt64
	Active                                                     bool
	Tags                                                       []string
}
type pageData struct {
	Repos                        []repoRow
	Page, Pages, Total, PageSize int
	Language, Active, Tag, Sort  string
	Languages, Tags              []string
	PrevURL, NextURL             string
}

var dashboard = template.Must(template.New("dashboard").Funcs(template.FuncMap{"minus1": func(v int) int { return v - 1 }, "plus1": func(v int) int { return v + 1 }}).Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>StarBase</title><style>
:root{color-scheme:dark;--bg:#10151d;--panel:#18212d;--line:#2d3948;--muted:#a5b1c2;--accent:#80d0c7}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:#e8eef5;font:15px/1.5 system-ui,sans-serif}header{padding:28px max(24px,calc((100% - 1160px)/2));background:linear-gradient(120deg,#1d3041,#17212e);border-bottom:1px solid var(--line)}h1{margin:0;font-size:30px}header p{margin:4px 0 0;color:var(--muted)}main{max-width:1160px;margin:24px auto;padding:0 20px}.filters{display:flex;gap:10px;flex-wrap:wrap;margin-bottom:18px}select,button{background:var(--panel);border:1px solid var(--line);color:inherit;border-radius:9px;padding:9px 12px}button{cursor:pointer}.repo{background:var(--panel);border:1px solid var(--line);border-radius:14px;margin:12px 0;padding:17px 18px;display:grid;grid-template-columns:1fr auto;gap:12px}.repo h2{font-size:19px;margin:0}.repo h2 a{color:var(--accent);text-decoration:none}.desc{color:var(--muted);margin:7px 0 12px}.stats{display:flex;gap:15px;flex-wrap:wrap;font-size:13px;color:#d4deea}.chip{border:1px solid #3d5263;background:#213240;border-radius:99px;padding:2px 9px;font-size:12px;margin:2px;color:#b8e2dc}.inactive{color:#ffbf8d}.active{color:#8fe0a4}.pager{display:flex;justify-content:center;gap:14px;align-items:center;margin:22px}.pager a{color:var(--accent)}dialog{max-width:850px;width:calc(100% - 30px);background:#17212d;color:#e8eef5;border:1px solid var(--line);border-radius:16px;padding:22px}dialog::backdrop{background:#000b}.close{float:right}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}.box{background:#101923;padding:14px;border-radius:10px;overflow-wrap:anywhere}.box h3{margin:0 0 8px;color:var(--accent);font-size:14px}.muted{color:var(--muted)}pre{white-space:pre-wrap;max-height:220px;overflow:auto}@media(max-width:600px){.repo{grid-template-columns:1fr}.repo button{justify-self:start}}
</style></head><body><header><h1>✦ StarBase</h1><p>Your starred repositories, enriched with activity, star history, LOC and Jev tags.</p></header><main><form class="filters"><select name="language"><option value="">All languages</option>{{range .Languages}}<option {{if eq $.Language .}}selected{{end}}>{{.}}</option>{{end}}</select><select name="active"><option value="">Any activity</option><option value="1" {{if eq .Active "1"}}selected{{end}}>Active</option><option value="0" {{if eq .Active "0"}}selected{{end}}>Inactive</option></select><select name="tag"><option value="">All tags</option>{{range .Tags}}<option {{if eq $.Tag .}}selected{{end}}>{{.}}</option>{{end}}</select><select name="sort"><option value="recent" {{if eq .Sort "recent"}}selected{{end}}>Recently starred</option><option value="stars" {{if eq .Sort "stars"}}selected{{end}}>Most stars</option><option value="activity" {{if eq .Sort "activity"}}selected{{end}}>Activity score</option><option value="loc" {{if eq .Sort "loc"}}selected{{end}}>Lines of code</option></select><button>Apply filters</button></form>{{if .Repos}}{{range .Repos}}<article class="repo"><div><h2><a href="{{.URL}}" target="_blank" rel="noopener">{{.Name}}</a></h2><div class="desc">{{if .Description}}{{.Description}}{{else}}No description{{end}}</div><div class="stats"><span>★ {{.Stars}}</span><span>{{if .Language}}{{.Language}}{{else}}Unknown language{{end}}</span><span class="{{if .Active}}active{{else}}inactive{{end}}">{{if .Active}}Active{{else}}Inactive{{end}} · {{.Activity}} commits / 60d</span><span>LOC {{if .LOC.Valid}}{{.LOC.Int64}}{{else}}—{{end}}</span><span>Starred {{.Starred}}</span></div><div>{{range .Tags}}<span class="chip">{{.}}</span>{{end}}</div></div><button onclick="details({{.ID}})">Details</button></article>{{end}}{{else}}<p class="muted">No repositories match these filters.</p>{{end}}<nav class="pager">{{if gt .Page 1}}<a href="{{.PrevURL}}">← Previous</a>{{end}}<span>Page {{.Page}} of {{.Pages}} · {{.Total}} repositories</span>{{if lt .Page .Pages}}<a href="{{.NextURL}}">Next →</a>{{end}}</nav></main><dialog id="detail"><button class="close" onclick="document.querySelector('#detail').close()">Close</button><div id="detailbody"></div></dialog><script>
async function details(id){const d=document.querySelector('#detail'),b=document.querySelector('#detailbody');b.innerHTML='Loading…';d.showModal();try{const r=await fetch('/repo/'+id);if(!r.ok)throw Error('Could not load details');const x=await r.json();const esc=s=>String(s??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));b.innerHTML='<h2>'+esc(x.full_name)+'</h2><p class="muted">'+esc(x.description)+'</p><div class="grid"><section class="box"><h3>Repository</h3><div>Stars '+esc(x.stars)+' · Forks '+esc(x.forks)+' · License '+esc(x.license||'—')+'</div><p>'+esc(x.created_at)+' created · pushed '+esc(x.pushed_at)+'</p><div>Tags: '+esc((x.tags||[]).join(', ')||'—')+'</div></section><section class="box"><h3>Activity</h3><div>'+esc(x.activity_score)+' commits in the last 60 days — '+(x.is_active?'Active':'Inactive')+'</div></section><section class="box"><h3>LOC · '+esc(x.loc_status)+'</h3><div>Total '+esc(x.loc_total??'Unavailable')+'</div><pre>'+esc(JSON.stringify(x.loc_by_language||{},null,2))+'</pre></section><section class="box"><h3>Star history · '+esc(x.star_history_status)+'</h3><div>Weekly star history points: '+esc((x.star_history||[]).length)+'</div><pre>'+esc(JSON.stringify(x.star_history||[],null,2))+'</pre></section><section class="box"><h3>Languages</h3><pre>'+esc(JSON.stringify(x.languages||{},null,2))+'</pre></section><section class="box"><h3>Description and README excerpt</h3><pre>'+esc(x.readme_excerpt||'No README excerpt available.')+'</pre></section></div>'}catch(e){b.textContent=e.message}}
</script></body></html>`))

func serve(db *sql.DB, addr string, pageSize int) error {
	t := dashboard
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		d, e := loadPage(db, r, pageSize)
		if e != nil {
			http.Error(w, e.Error(), 500)
			return
		}
		if e = t.Execute(w, d); e != nil {
			http.Error(w, "render failed", 500)
		}
	})
	mux.HandleFunc("GET /repo/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if e != nil {
			http.Error(w, "invalid id", 400)
			return
		}
		detail, e := loadDetail(db, id)
		if e == sql.ErrNoRows {
			http.NotFound(w, r)
			return
		}
		if e != nil {
			http.Error(w, "could not load repository", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(detail)
	})
	fmt.Println("StarBase listening at http://" + addr)
	return http.ListenAndServe(addr, mux)
}
func loadPage(db *sql.DB, r *http.Request, size int) (pageData, error) {
	q := r.URL.Query()
	p, _ := strconv.Atoi(q.Get("page"))
	if p < 1 {
		p = 1
	}
	lang, active, tag, sortKey := q.Get("language"), q.Get("active"), q.Get("tag"), q.Get("sort")
	switch sortKey {
	case "stars", "activity", "loc":
	default:
		sortKey = "recent"
	}
	order := map[string]string{"recent": "starred_at DESC", "stars": "stars DESC, starred_at DESC", "activity": "activity_score DESC, starred_at DESC", "loc": "loc_total DESC, starred_at DESC"}[sortKey]
	where := []string{"1=1"}
	args := []any{}
	if lang != "" {
		where = append(where, "EXISTS(SELECT 1 FROM json_each(languages_json) WHERE key=?)")
		args = append(args, lang)
	}
	if active == "1" || active == "0" {
		where = append(where, "is_active=?")
		args = append(args, active)
	}
	if tag != "" {
		where = append(where, "EXISTS(SELECT 1 FROM repository_tags t WHERE t.repository_id=repositories.id AND t.tag=?)")
		args = append(args, tag)
	}
	w := strings.Join(where, " AND ")
	var total int
	if e := db.QueryRow(`SELECT count(*) FROM repositories WHERE `+w, args...).Scan(&total); e != nil {
		return pageData{}, e
	}
	pages := (total + size - 1) / size
	if pages == 0 {
		pages = 1
	}
	if p > pages {
		p = pages
	}
	rows, e := db.Query(`SELECT id,full_name,description,html_url,primary_language,starred_at,pushed_at,license_name,stars,activity_score,loc_total,is_active FROM repositories WHERE `+w+` ORDER BY `+order+` LIMIT ? OFFSET ?`, append(args, size, (p-1)*size)...)
	if e != nil {
		return pageData{}, e
	}
	defer rows.Close()
	var repos []repoRow
	for rows.Next() {
		var x repoRow
		if e = rows.Scan(&x.ID, &x.Name, &x.Description, &x.URL, &x.Language, &x.Starred, &x.Pushed, &x.License, &x.Stars, &x.Activity, &x.LOC, &x.Active); e != nil {
			return pageData{}, e
		}
		repos = append(repos, x)
	}
	if e = rows.Err(); e != nil {
		return pageData{}, e
	}
	// Close the page query before asking SQLite for each repository's tags.
	// openDB limits the pool to one connection, so nested queries here would
	// wait forever for the connection still held by rows.
	if e = rows.Close(); e != nil {
		return pageData{}, e
	}
	for i := range repos {
		repos[i].Tags, e = getTags(db, repos[i].ID)
		if e != nil {
			return pageData{}, e
		}
	}
	languages := []string{}
	lr, e := db.Query(`SELECT DISTINCT j.key FROM repositories, json_each(repositories.languages_json) AS j ORDER BY j.key`)
	if e != nil {
		return pageData{}, e
	}
	for lr.Next() {
		var s string
		if e = lr.Scan(&s); e != nil {
			lr.Close()
			return pageData{}, e
		}
		languages = append(languages, s)
	}
	lr.Close()
	tags := []string{}
	tr, e := db.Query(`SELECT DISTINCT tag FROM repository_tags ORDER BY tag`)
	if e != nil {
		return pageData{}, e
	}
	for tr.Next() {
		var s string
		if e = tr.Scan(&s); e != nil {
			tr.Close()
			return pageData{}, e
		}
		tags = append(tags, s)
	}
	tr.Close()
	prev := pageQuery(r, p-1)
	next := pageQuery(r, p+1)
	return pageData{repos, p, pages, total, size, lang, active, tag, sortKey, languages, tags, prev, next}, nil
}
func pageQuery(r *http.Request, page int) string {
	q := r.URL.Query()
	q.Set("page", strconv.Itoa(page))
	return "?" + q.Encode()
}
func getTags(db *sql.DB, id int64) ([]string, error) {
	rows, e := db.Query(`SELECT tag FROM repository_tags WHERE repository_id=? ORDER BY probability DESC,tag`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if e = rows.Scan(&s); e != nil {
			return nil, e
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
func loadDetail(db *sql.DB, id int64) (map[string]any, error) {
	var name, desc, html, lang, langs, topics, license, created, pushed, excerpt, history, historyStatus, locLang, locTree, locStatus string
	var stars, forks, activity int
	var active bool
	var loc sql.NullInt64
	e := db.QueryRow(`SELECT full_name,description,html_url,primary_language,languages_json,topics_json,license_name,created_at,pushed_at,readme_excerpt,stars,forks,activity_score,is_active,loc_total,loc_by_language_json,loc_tree_json,loc_status,star_history_json,star_history_status FROM repositories WHERE id=?`, id).Scan(&name, &desc, &html, &lang, &langs, &topics, &license, &created, &pushed, &excerpt, &stars, &forks, &activity, &active, &loc, &locLang, &locTree, &locStatus, &history, &historyStatus)
	if e != nil {
		return nil, e
	}
	var tagList []string
	tagList, e = getTags(db, id)
	if e != nil {
		return nil, e
	}
	decode := func(s string) any { var v any; _ = json.Unmarshal([]byte(s), &v); return v }
	var locValue any
	if loc.Valid {
		locValue = loc.Int64
	}
	return map[string]any{"full_name": name, "description": desc, "html_url": html, "primary_language": lang, "languages": decode(langs), "topics": decode(topics), "license": license, "created_at": created, "pushed_at": pushed, "readme_excerpt": excerpt, "stars": stars, "forks": forks, "activity_score": activity, "is_active": active, "loc_total": locValue, "loc_by_language": decode(locLang), "loc_tree": decode(locTree), "loc_status": locStatus, "star_history": decode(history), "star_history_status": historyStatus, "tags": tagList}, nil
}
