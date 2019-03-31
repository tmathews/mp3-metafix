package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bogem/id3v2"
	"github.com/zmb3/spotify"
	"golang.org/x/oauth2/clientcredentials"
)

var (
	ClientId     = ""
	ClientSecret = ""
)

func main() {
	var filename string
	var genre string
	var opts Opts

	flag.StringVar(&genre, "genre", "", "Provide a default genre(s) (comma separated)")
	flag.StringVar(&opts.Term, "search", "", "Override default term search")
	flag.BoolVar(&opts.Reset, "reset", false, "Reset all metadata")
	flag.BoolVar(&opts.Rename, "rename", false, "Rename files to 'Artist - Title.mp3' format")
	flag.Parse()

	filename = flag.Arg(0)
	opts.Genres = CommaString(genre)

	fi, err := os.Stat(filename)
	if err != nil {
		panic(err)
	}

	config := &clientcredentials.Config{
		ClientID:     ClientId,
		ClientSecret: ClientSecret,
		TokenURL:     spotify.TokenURL,
	}
	token, err := config.Token(context.Background())
	if err != nil {
		panic(err)
	}
	client := spotify.Authenticator{}.NewClient(token)

	if fi.IsDir() {
		opts.Term = ""
		err = DoDirectory(&client, filename, opts)
	} else {
		ext := filepath.Ext(filename)
		if strings.ToLower(ext) != ".mp3" {
			err = errors.New("not mp3 file")
		} else {
			err = DoOne(&client, filename, opts)
		}
	}
	if err != nil {
		panic(err)
	}
}

func DoDirectory(client *spotify.Client, filename string, opts Opts) error {
	opts.Term = ""
	files, err := ioutil.ReadDir(filename)
	if err != nil {
		return err
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		ext := filepath.Ext(file.Name())
		if strings.ToLower(ext) != ".mp3" {
			continue
		}
		if err := DoOne(client, filepath.Join(filename, file.Name()), opts); err != nil {
			fmt.Println(err.Error())
		} else {
			fmt.Printf("\n\n")
		}
	}
	return nil
}

func DoOne(client *spotify.Client, filename string, opts Opts) error {
	fmt.Printf("%s\n", filename)
	search := opts.Term
	if search == "" {
		search = TermFromFilename(filename)
	}

	reader := bufio.NewReader(os.Stdin)
	infos := make([]*Info, 0)

	for len(infos) == 0 {
		fmt.Printf("Searching '%s'\n", search)
		var err error
		infos, err = SearchTracks(client, search)
		if err != nil {
			return err
		}
		if len(infos) == 0 {
			fmt.Printf("No results, enter new term (or continue 'q'): ")
			choice, _ := reader.ReadString('\n')
			if choice == "q\n" {
				return nil
			}
			search = strings.TrimSpace(choice)
		}
	}

	for _, info := range infos {
		fmt.Printf("%s\n\n", info.String())
	}
	var info *Info

	// Wait for a valid choice
	for info == nil {
		fmt.Printf("Choose 1 - %d (or continue 'q'): ", len(infos))
		choiceStr, _ := reader.ReadString('\n')
		if choiceStr == "q\n" {
			return nil
		}

		choice, _ := strconv.Atoi(strings.TrimSpace(choiceStr))
		for _, x := range infos {
			if x.Index == choice {
				info = x
			}
		}
		if info == nil {
			fmt.Printf("Not a choice\n")
		}
	}

	// Figure out genre
	if len(info.Genres) == 0 {
		if len(opts.Genres) == 0 {
			fmt.Printf("Enter Genre(s): ")
			x, _ := reader.ReadString('\n')
			info.Genres = CommaString(x)
		} else {
			info.Genres = opts.Genres
		}
	}

	if err := WriteTags(info, filename, opts.Reset); err != nil {
		return err
	}

	if opts.Rename {
		dir := filepath.Dir(filename)
		ext := filepath.Ext(filename)
		err := os.Rename(filename, filepath.Join(dir, fmt.Sprintf("%s - %s%s", strings.Join(info.ArtistNames, ", "), info.TrackTitle, ext)))
		if err != nil {
			fmt.Printf("Failed to rename file: %s\n", err.Error())
		}
	}

	return nil
}

type Opts struct {
	Term   string
	Genres []string
	Reset  bool
	Rename bool
}

type Info struct {
	AlbumTitle  string
	AlbumType   string
	ArtistNames []string
	Copyright   []string
	CoverURL    string
	DiscNumber  int
	Duration    int
	Genres      []string
	Index       int
	Publishing  []string
	ReleaseDate time.Time
	TrackNumber int
	TrackTitle  string
	URL         string
}

func (i *Info) String() string {
	return fmt.Sprintf("[%d]\nName: %s\nAlbum: %s (%s)\nArtists: %s\nRelease Date: %s\nGenres: %s\nCopyright: %s\nPublishing: %s\nCover URL: %s\nTrack Number: %d\nDisc Number: %d\nDuration: %f\nURL: %s",
		i.Index,
		i.TrackTitle,
		i.AlbumTitle, i.AlbumType,
		strings.Join(i.ArtistNames, ", "),
		i.ReleaseDate.Format("2006-01-02"),
		strings.Join(i.Genres, ", "),
		strings.Join(i.Copyright, ", "),
		strings.Join(i.Publishing, ", "),
		i.CoverURL,
		i.TrackNumber,
		i.DiscNumber,
		i.Duration,
		i.URL)
}

func TermFromFilename(a string) string {
	a = filepath.Base(a)
	ext := filepath.Ext(a)
	return a[:len(a)-len(ext)]
}

func CommaString(a string) []string {
	if strings.TrimSpace(a) == "" {
		return []string{}
	}
	xs := strings.Split(a, ",")
	for i, x := range xs {
		xs[i] = strings.TrimSpace(x)
	}
	return xs
}

func WriteTags(info *Info, filename string, reset bool) error {
	tag, err := id3v2.Open(filename, id3v2.Options{Parse: true})
	if err != nil {
		panic(err)
	}
	defer tag.Close()

	if reset {
		tag.DeleteAllFrames()
	}

	// Set the cover art!
	if info.CoverURL != "" {
		req, err := http.Get(info.CoverURL)
		if err != nil {
			return err
		}
		buf, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return err
		}
		tag.AddAttachedPicture(id3v2.PictureFrame{
			Encoding:    id3v2.EncodingUTF8,
			MimeType:    "image/jpeg",
			PictureType: id3v2.PTFrontCover,
			Description: "Front cover",
			Picture:     buf,
		})
	}

	tag.SetVersion(4)
	// Always delete these?
	//tag.DeleteFrames(tag.CommonID("Unsynchronised lyrics/text transcription"))
	//tag.DeleteFrames(tag.CommonID("Encoded by"))
	tag.SetTitle(info.TrackTitle)
	tag.SetAlbum(info.AlbumTitle)
	tag.SetArtist(strings.Join(info.ArtistNames, ", "))
	tag.SetGenre(strings.Join(info.Genres, ", "))
	tag.SetYear(info.ReleaseDate.Format("2006-01-02"))
	tag.AddTextFrame(tag.CommonID("Track number/Position in set"), tag.DefaultEncoding(), fmt.Sprintf("%d", info.TrackNumber))
	tag.AddTextFrame(tag.CommonID("Part of a set"), tag.DefaultEncoding(), fmt.Sprintf("%d", info.DiscNumber))
	tag.AddTextFrame(tag.CommonID("Date"), tag.DefaultEncoding(), info.ReleaseDate.Format("2006-01-02"))
	tag.AddTextFrame(tag.CommonID("Length"), tag.DefaultEncoding(), strconv.Itoa(info.Duration))
	tag.AddTextFrame(tag.CommonID("Publisher"), tag.DefaultEncoding(), strings.Join(info.Publishing, ", "))
	tag.AddTextFrame(tag.CommonID("Copyright message"), tag.DefaultEncoding(), strings.Join(info.Copyright, ", "))
	tag.AddCommentFrame(id3v2.CommentFrame{
		Encoding:    tag.DefaultEncoding(),
		Language:    "eng",
		Description: "Spotify URL",
		Text:        fmt.Sprintf("Spotify URL: %s", info.URL),
	})
	tag.AddTextFrame("WCOM", tag.DefaultEncoding(), info.URL)

	return tag.Save()
}

func SearchTracks(client *spotify.Client, term string) ([]*Info, error) {
	res, err := client.Search(term, spotify.SearchTypeTrack)
	if err != nil {
		return nil, err
	}
	infos := make([]*Info, 0)
	for i, track := range res.Tracks.Tracks {
		names := make([]string, 0)
		copyrights := make([]string, 0)
		publishing := make([]string, 0)
		for _, artist := range track.Artists {
			names = append(names, artist.Name)
		}

		album, err := client.GetAlbum(track.Album.ID)
		if err != nil {
			continue
		}
		for _, x := range album.Copyrights {
			if x.Type == "C" {
				copyrights = append(copyrights, x.Text)
			} else if x.Type == "P" {
				publishing = append(publishing, x.Text)
			}
		}

		rdate, _ := time.Parse("2006-01-02", album.ReleaseDate)
		info := Info{
			Index:       i + 1,
			TrackTitle:  track.Name,
			AlbumTitle:  album.Name,
			AlbumType:   album.AlbumType,
			ArtistNames: names,
			ReleaseDate: rdate,
			Genres:      album.Genres,
			Copyright:   copyrights,
			Publishing:  publishing,
			CoverURL:    album.Images[0].URL,
			TrackNumber: track.TrackNumber,
			DiscNumber:  track.DiscNumber,
			Duration:    track.Duration,
			URL:         track.ExternalURLs["spotify"],
		}
		infos = append(infos, &info)
	}
	return infos, nil
}
