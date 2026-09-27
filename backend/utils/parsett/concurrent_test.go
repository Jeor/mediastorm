package parsett

import (
	"fmt"
	"testing"
)

func TestConcurrentTitleParsing(t *testing.T) {
	for i := 0; i < 12; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			for attempt := 0; attempt < 10; attempt++ {
				const title = "Moana.2016.1080p.WEB-DL.POLISH.AAC"
				parsed, err := ParseTitle(title)
				if err != nil || parsed.Title != "Moana" || parsed.Year != 2016 || parsed.Resolution != "1080p" {
					t.Fatalf("single parse=%+v, %v", parsed, err)
				}
				batch, err := ParseTitleBatch([]string{title, "Vaiana.2016.720p.WEB-DL"})
				if err != nil || batch[title].Year != 2016 || batch["Vaiana.2016.720p.WEB-DL"].Resolution != "720p" {
					t.Fatalf("batch parse=%+v, %v", batch, err)
				}
			}
		})
	}
}
