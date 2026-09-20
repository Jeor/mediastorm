package filter

import (
	"testing"

	"novastream/models"
)

func TestResults_MovieInstallmentIdentity(t *testing.T) {
	const original = "Winnie the Pooh: Blood and Honey"
	const sequel = "Winnie-the-Pooh: Blood and Honey 2"
	const wrongRelease = "Winnie-the-Pooh.Blood.and.Honey.2.2024.2160p.UHD.BluRay.DTS-HD.MA.5.1.DV.HDR.x265-j3rico"
	for _, tt := range []struct {
		name, title, release string
		year                 int
		alternate            []string
		want                 bool
	}{
		{"reported sequel", original, wrongRelease, 2023, nil, false},
		{"unknown year", original, wrongRelease, 0, nil, false},
		{"release without year", original, "Winnie.the.Pooh.Blood.and.Honey.2.2160p.BluRay.x265", 2023, nil, false},
		{"original", original, "Winnie.the.Pooh.Blood.and.Honey.2023.2160p.BluRay.x265", 2023, nil, true},
		{"year tolerance", original, "Winnie.the.Pooh.Blood.and.Honey.2024.2160p.BluRay.x265", 2023, nil, true},
		{"sequel requested", sequel, wrongRelease, 2024, nil, true},
		{"original for sequel", sequel, "Winnie.the.Pooh.Blood.and.Honey.2023.2160p.BluRay.x265", 2024, nil, false},
		{"different sequel", sequel, "Winnie.the.Pooh.Blood.and.Honey.3.2024.2160p.BluRay.x265", 2024, nil, false},
		{"roman sequel", original, "Winnie.the.Pooh.Blood.and.Honey.II.2024.2160p.BluRay.x265", 2023, nil, false},
		{"localized title", original, "Ursinho.Pooh.Sangue.e.Mel.2023.1080p.BluRay.x264", 2023, []string{"Ursinho Pooh: Sangue e Mel"}, true},
		{"localized sequel", original, "Ursinho.Pooh.Sangue.e.Mel.2.2024.1080p.BluRay.x264", 2023, []string{"Ursinho Pooh: Sangue e Mel"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := Results([]models.NZBResult{{Title: tt.release}}, Options{
				ExpectedTitle: tt.title, ExpectedYear: tt.year, AlternateTitles: tt.alternate, IsMovie: true,
			})
			if (len(got) == 1) != tt.want {
				t.Fatalf("release accepted = %v, want %v", len(got) == 1, tt.want)
			}
		})
	}
}
