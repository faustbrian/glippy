package report

import "testing"

func TestMarshalJSONEmitsStableVersionedEnvelope(t *testing.T) {
	t.Parallel()

	result := NewFormatResult(
		"check",
		"findings",
		1,
		1,
		1,
		true,
		[]File{{Path: "/project/source.go", Status: FileDifferent}},
		nil,
	)

	got, err := MarshalJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n" +
		"  \"schema_version\": 1,\n" +
		"  \"command\": \"fmt\",\n" +
		"  \"mode\": \"check\",\n" +
		"  \"outcome\": {\n" +
		"    \"category\": \"findings\",\n" +
		"    \"exit_code\": 1\n" +
		"  },\n" +
		"  \"summary\": {\n" +
		"    \"files\": 1,\n" +
		"    \"changed\": 1,\n" +
		"    \"complete\": true\n" +
		"  },\n" +
		"  \"files\": [\n" +
		"    {\n" +
		"      \"path\": \"/project/source.go\",\n" +
		"      \"status\": \"different\"\n" +
		"    }\n" +
		"  ],\n" +
		"  \"errors\": []\n" +
		"}\n"
	if string(got) != want {
		t.Fatalf("MarshalJSON() =\n%s\nwant:\n%s", got, want)
	}
}
