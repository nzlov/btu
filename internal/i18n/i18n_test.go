package i18n

import (
	"reflect"
	"regexp"
	"sort"
	"testing"
)

func TestFromLANG(t *testing.T) {
	tests := []struct {
		lang    string
		chinese bool
	}{
		{lang: "zh_CN.UTF-8", chinese: true},
		{lang: "zh-TW", chinese: true},
		{lang: "zh", chinese: true},
		{lang: "en_US.UTF-8"},
		{lang: "C.UTF-8"},
		{lang: ""},
	}
	for _, test := range tests {
		t.Run(test.lang, func(t *testing.T) {
			if got := FromLANG(test.lang).IsChinese(); got != test.chinese {
				t.Fatalf("IsChinese() = %v, want %v", got, test.chinese)
			}
		})
	}
}

func TestCatalogsHaveMatchingKeys(t *testing.T) {
	for key, english := range catalogs[English] {
		if english == "" && key != HelpTemplate {
			t.Errorf("English message %q is empty", key)
		}
		if _, found := catalogs[Chinese][key]; !found {
			t.Errorf("Chinese catalog is missing %q", key)
		}
	}
	for key := range catalogs[Chinese] {
		if _, found := catalogs[English][key]; !found {
			t.Errorf("English catalog is missing %q", key)
		}
	}
}

func TestCatalogFormatVerbsMatch(t *testing.T) {
	verbPattern := regexp.MustCompile(`%(?:\[\d+\])?[-+#0 .\d]*[a-zA-Z%]`)
	verbs := func(value string) []byte {
		matches := verbPattern.FindAllString(value, -1)
		result := make([]byte, len(matches))
		for index, match := range matches {
			result[index] = match[len(match)-1]
		}
		sort.Slice(result, func(first, second int) bool { return result[first] < result[second] })
		return result
	}
	for key, english := range catalogs[English] {
		if chinese := catalogs[Chinese][key]; !reflect.DeepEqual(verbs(english), verbs(chinese)) {
			t.Errorf("format verbs differ for %q: en=%q zh=%q", key, english, chinese)
		}
	}
}

func TestLocalizerSelectsCatalog(t *testing.T) {
	if got := EnglishLocalizer().Text(HelpShow); got != "show help" {
		t.Fatalf("English text = %q", got)
	}
	if got := FromLANG("zh_CN.UTF-8").Text(HelpShow); got != "显示帮助" {
		t.Fatalf("Chinese text = %q", got)
	}
}
