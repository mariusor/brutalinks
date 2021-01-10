package app

import (
	"bytes"
	"fmt"
	pub "github.com/go-ap/activitypub"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const TagMention = "mention"
const TagTag = "tag"

type Tag struct {
	Hash        Hash          `json:"hash"`
	Type        string        `json:"-"`
	Name        string        `json:"name,omitempty"`
	URL         string        `json:"id,omitempty"`
	SubmittedAt time.Time     `json:"-"`
	SubmittedBy *Account      `json:"by,omitempty"`
	UpdatedAt   time.Time     `json:"-"`
	UpdatedBy   *Account      `json:"-"`
	Metadata    *ItemMetadata `json:"-"`
	pub         pub.Item      `json:"-"`
}

type TagCollection []Tag

func (c TagCollection) Contains(t Tag) bool {
	for _, tt := range c {
		if tt.Type == t.Type && tt.Name == t.Name && tt.URL == t.URL {
			return true
		}
	}
	return false
}

func mimeTypeTagReplace(m string, t Tag) string {
	var cls string

	if t.Type == TagTag {
		cls = "tag"
	}
	if t.Type == TagMention {
		cls = "mention"
	}

	name := t.Name
	if len(name) > 1 && name[0] == '#' {
		name = name[1:]
	}
	return fmt.Sprintf("<a href='%s' rel='%s'>%s</a>", t.URL, cls, name)
}

func replaceTag(d []byte, t Tag, w string) []byte {
	inWord := func(d []byte, i, end int) bool {
		dl := len(d)
		if i < 1 || end > dl {
			return false
		}
		before := isWordDelimiter(d[i-1])
		after := true
		if end < dl {
			after = isWordDelimiter(d[end])
		}
		return before && after
	}

	var base []string
	base = append(base, t.Name)
	if u, err := url.Parse(t.URL); err == nil && len(u.Host) > 0 {
		base = append(base, t.Name+`@`+u.Host)
	}

	var pref [][]byte
	if t.Type == TagMention {
		pref = [][]byte{{'~'}, {'@'}}
	} else {
		pref = [][]byte{{'#'}}
	}
	var search [][]byte
	for _, p := range pref {
		for _, b := range base {
			s := append(p, b...)
			search = append(search, s)
		}
	}
	for _, s := range search {
		end := 0
		for {
			if end >= len(d) {
				break
			}
			inx := bytes.Index(d[end:], s)
			if inx < 0 {
				break
			}
			pos := end + inx
			end = pos + len(s)
			if end > len(d) {
				break
			}
			if !inWord(d, pos, end) {
				d = replaceBetweenPos(d, []byte(w), pos, end)
			}
		}
	}
	return d
}

func replaceTags(mime string, r HasContent) string {
	dataMap := r.Content()
	if len(dataMap) == 0 {
		return ""
	}

	var data []byte
	for m, d := range dataMap {
		if strings.ToLower(mime) == strings.ToLower(m) {
			data = d
			break
		}
	}
	if len(data) == 0 {
		return ""
	}
	if len(r.Tags())+len(r.Mentions()) == 0 {
		return string(data)
	}

	replaces := make(map[string]string, 0)
	tags := r.Tags()
	if tags != nil {
		for _, t := range tags {
			name := t.Name
			if len(name) > 1 && name[0] != '#' {
				name = fmt.Sprintf("#%s", name)
			}
			if inRange(name, replaces) {
				continue
			}
			replaces[name] = mimeTypeTagReplace(mime, t)
		}
	}
	mentions := r.Mentions()
	if mentions != nil {
		for idx, t := range mentions {
			lbl := fmt.Sprintf(":::MENTION_%d:::", idx)
			if inRange(lbl, replaces) {
				continue
			}
			data = replaceTag(data, t, lbl)
			replaces[lbl] = mimeTypeTagReplace(mime, t)
		}
	}
	for to, repl := range replaces {
		data = bytes.ReplaceAll(data, []byte(to), []byte(repl))
	}
	return string(data)
}

func loadTags(data string) (TagCollection, TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(TagCollection, 0)
	mentions := make(TagCollection, 0)

	r := regexp.MustCompile(`(?:\A|\s)((?:[~@]\w+)(?:@\w+.\w+)?|(?:#[\w-]{3,}))`)
	matches := r.FindAllSubmatch([]byte(data), -1)

	for _, sub := range matches {
		t := getTagFromBytes(sub[1])
		if t.Type == TagMention && !mentions.Contains(t) {
			mentions = append(mentions, t)
		}
		if t.Type == TagTag && !tags.Contains(t) {
			tags = append(tags, t)
		}
	}
	return tags, mentions
}

func getTagFromBytes(d []byte) Tag {
	var name, host []byte
	t := Tag{}

	if ind := bytes.LastIndex(d, []byte{'@'}); ind > 1 {
		name = d[1:ind]
		host = []byte(fmt.Sprintf("https://%s", d[ind+1:]))
	} else {
		name = d[1:]
		host = []byte(Instance.BaseURL)
	}
	if d[0] == '@' || d[0] == '~' {
		// mention
		t.Type = TagMention
		t.Name = string(name)
		t.URL = fmt.Sprintf("%s/~%s", host, name)
	}
	if d[0] == '#' {
		// @todo(marius) :link_generation: make the tag links be generated from the corresponding route
		t.Type = TagTag
		t.Name = string(name)
		t.URL = fmt.Sprintf("%s/t/%s", host, name)
	}
	return t
}
