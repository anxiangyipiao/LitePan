package rss

import "testing"

func TestExtractEpisode(t *testing.T) {
	cases := []struct {
		title string
		want  EpisodeRange
	}{
		{"孤独摇滚 第2话 1080p", EpisodeRange{2, 2, true}},
		{"间谍过家家 第2話 720p", EpisodeRange{2, 2, true}},
		{"某番 第12集", EpisodeRange{12, 12, true}},
		{"Title EP12", EpisodeRange{12, 12, true}},
		{"Title Ep. 12 [1080p]", EpisodeRange{12, 12, true}},
		{"Title [01]", EpisodeRange{1, 1, true}},
		{"Title（02）", EpisodeRange{2, 2, true}},
		{"Title (03) 1080p", EpisodeRange{3, 3, true}},
		{"Some Anime - 01", EpisodeRange{1, 1, true}},
		{"Some Anime - 01-12", EpisodeRange{1, 12, true}},
		{"Anime 2話", EpisodeRange{2, 2, true}},
		{"Anime 第12345话", EpisodeRange{12345, 12345, true}},

		// 歧义/年份/画质必须排除
		{"Some Anime (2024)", EpisodeRange{}},
		{"[1080p] Anime", EpisodeRange{}},
		{"Anime 1080p", EpisodeRange{}},
		{"Anime 2160p [WEB]", EpisodeRange{}},
		{"Anime", EpisodeRange{}},
		{"Movie 剧场版", EpisodeRange{}},
	}
	for _, c := range cases {
		got := ExtractEpisode(c.title)
		if got != c.want {
			t.Errorf("ExtractEpisode(%q) = %+v, want %+v", c.title, got, c.want)
		}
	}
}
