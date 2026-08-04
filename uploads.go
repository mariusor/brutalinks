package brutalinks

import (
	"encoding/base64"
	"io"
	"io/fs"
	"strings"

	ct "github.com/elnormous/contenttype"
	vocab "github.com/go-ap/activitypub"
)

func getActivityObjectType(t vocab.MimeType) vocab.ActivityVocabularyType {
	mt, err := ct.ParseMediaType(string(t))
	if err != nil {
		return ""
	}

	switch mt.Type {
	case "text":
		switch mt.Subtype {
		case "css":
			return vocab.DocumentType
		}
		return vocab.NoteType
	case "image":
		return vocab.ImageType
	case "audio":
		return vocab.AudioType
	case "video":
		return vocab.VideoType
	case "application":
		switch mt.Subtype {
		case "pdf":
			return vocab.DocumentType
		case "svg":
			return vocab.DocumentType
		}
	}
	return ""
}
func nl[T ~string](s T) vocab.NaturalLanguageValues {
	return vocab.DefaultNaturalLanguage(s)
}

var enc = base64.RawStdEncoding

func base64Content(raw []byte, contType string) string {
	s := strings.Builder{}
	s.WriteString("data:")
	s.WriteString(contType)
	s.WriteString(";base64,")

	data := make([]byte, enc.EncodedLen(len(raw)))
	enc.Encode(data, raw)
	s.Write(data)

	return s.String()
}

func loadContentFromFile(file fs.File, ob *vocab.Object) {
	if raw, _ := io.ReadAll(file); len(raw) > 0 {
		ob.Content = nl(base64Content(raw, string(ob.MediaType)))
	}
}
