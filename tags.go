package brutalinks

import (
	"bytes"
	"fmt"
	"html/template"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	vocab "github.com/go-ap/activitypub"
)

const (
	TagMention = "mention"
	TagTag     = "tag"

	tagNameModerator = "#mod"
	tagNameSysOP     = "#sysop"
)

type Tag struct {
	Hash        Hash          `json:"hash"`
	Type        string        `json:"-"`
	Name        string        `json:"name,omitempty"`
	URL         string        `json:"url,omitempty"`
	SubmittedAt time.Time     `json:"-"`
	SubmittedBy *Account      `json:"-"`
	UpdatedAt   time.Time     `json:"-"`
	UpdatedBy   *Account      `json:"-"`
	Metadata    *ItemMetadata `json:"-"`
	Pub         vocab.Item    `json:"-"`
}

func (t Tag) IsLocal() bool {
	return strings.Contains(t.URL, Instance.BaseURL.String())
}

type TagCollection []Tag

func (c TagCollection) Contains(t Tag) bool {
	for _, tt := range c {
		if tt.Hash.IsValid() && tt.Hash.String() == t.Hash.String() {
			return true
		}
		if tt.Metadata != nil && t.Metadata != nil && tt.Metadata.ID == t.Metadata.ID {
			return true
		}
		if tt.Type == t.Type && tt.Name == t.Name && tt.URL == t.URL {
			return true
		}
	}
	return false
}

func renderParams(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	s := strings.Builder{}
	s.WriteString(" ")
	for k, v := range values {
		s.WriteString(k)
		s.WriteString(`=`)
		s.WriteString(`"`)
		s.WriteString(strings.Join(v, " "))
		s.WriteString(`"`)
	}
	return s.String()
}

func mimeTypeTagReplace(m string, t Tag) string {
	// TODO(marius): this should be put through the PermaLink and AccountHandle functions so the logic
	//   of federated vs. local actors is handled in a single place.
	params := make(url.Values)

	name := t.Name
	if t.Type == TagTag {
		params.Set("rel", "tag")
		if len(name) > 1 && name[0] == '#' {
			name = name[1:]
		}
	}
	if t.Type == TagMention {
		params.Set("rel", "mention")
		if !t.IsLocal() {
			params.Add("rel", "external")
			name = t.Name + "@" + host(t.URL)
		} else if t.URL == "" {
			// NOTE(marius) this is a kludge way of generating a local URL for an actor that belongs
			// to our main FedBOX instance
			t.URL = fmt.Sprintf("%s/~%s", Instance.BaseURL.String(), name)
		}
	}

	if Instance.ModTags.Contains(t) {
		params.Set("class", "")
	}

	return fmt.Sprintf(`<a href="%s"%s>%s</a>`, t.URL, renderParams(params), name)
}

func replaceRemoteMentions(d []byte, t Tag, w string) []byte {
	if t.Type != TagMention {
		return d
	}
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
	if !HostIsLocal(t.URL) {
		base = append(base, t.Name+`@`+host(t.URL))
	}
	if t.Metadata != nil && len(t.Metadata.ID) > 0 {
		base = append(base, t.Name+`@`+host(t.Metadata.ID))

	}
	base = append(base, t.Name)

	pref := [][]byte{{'~'}, {'@'}}
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

func renderTag(t Tag) template.HTML {
	return template.HTML(mimeTypeTagReplace("text/html", t))
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

	tags := r.Tags()
	sort.SliceStable(tags, func(i, j int) bool {
		return len(tags[i].Name) >= len(tags[j].Name)
	})
	for _, t := range tags {
		name := t.Name
		if len(name) > 1 && name[0] != '#' {
			t.Name = fmt.Sprintf("#%s", name)
		}
		data = bytes.ReplaceAll(data, []byte(t.Name), []byte(mimeTypeTagReplace(mime, t)))
	}
	for _, t := range r.Mentions() {
		data = replaceRemoteMentions(data, t, mimeTypeTagReplace(mime, t))
	}
	return string(data)
}

func stringSliceContains[T ~string](sl []T, value T) bool {
	for _, chk := range sl {
		if chk == value {
			return true
		}
	}
	return false
}

func loadTags(data string) (TagCollection, TagCollection) {
	if !strings.ContainsAny(data, "#@~") {
		return nil, nil
	}
	tags := make(TagCollection, 0)
	mentions := make(TagCollection, 0)

	r := regexp.MustCompile(`(?:\A|\s)((?:[~@]\w+)(?:@[\w-]+.?\w*)?|(?:#[\w-]{3,}))`)
	matches := r.FindAllSubmatch([]byte(data), -1)

	invalidTags := []string{"#include", "#define"}
	for _, sub := range matches {
		ttext := sub[1]
		if stringSliceContains(invalidTags, string(ttext)) {
			continue
		}
		t := getTagFromBytes(ttext)
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
		host = []byte(Instance.BaseURL.String())
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
