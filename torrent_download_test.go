//
// To run the unittest, run go test from the top folder
//
// To run a specific test case, use the -run option.
// $ go test -run TestCleanupOldFilesNothingToRemove
//
//

/*

Check out the anacrolyx/torrent github project

# Download a torrent file
go run ./cmd/torrent/ download --debug --addr localhost:42090 http://localhost:51297/foo.torrent

# Pretty print a torrent file
torrent$ go run cmd/torrent-metainfo-pprint/main.go < /var/folders/9g/8xh34btn3ygfzngrzj6xvv3w0000gp/T/TestTorrentDownload584422358/001/foo.torrent
{
  "Announce": "udp://127.0.0.1:6969",
  "AnnounceList": [
    [
      "udp://127.0.0.1:6969"
    ]
  ],
  "InfoHash": "6d504e3a1e43349a68af73e880d1b53ba7b307e0",
  "Name": "foo.txt",
  "NumFiles": 1,
  "NumPieces": 1,
  "PieceLength": 262144,
  "TotalLength": 18,
  "UrlList": null
}

# Announce a torrent, using its hash:

torrent$ go run ./cmd/torrent/ announce --infohash 7d504e3a1e43349a68af73e880d1b53ba7b307e0 udp://example.com:6969
(http.AnnounceResponse) {
 Interval: (int32) 60,
 Leechers: (int32) 0,
 Seeders: (int32) 1,
 Peers: ([]http.Peer) (len=1 cap=1) {
  (http.Peer) 10.30.0.230:42069
 }
}

# Chibaya torrent tracker project

Run with
./chihaya --config dist/example_config_udp_only.yaml

*/

package datamover

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/tracker"

	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
)

// Start a test HTTP file server
func CreateAndStartFileServer(rootDir string) (*http.Server, string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Server request: %s", r.URL.Path)
		http.ServeFile(w, r, rootDir+r.URL.Path)
	})

	// Get a free port by listening on port 0
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		panic(err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	portStr := fmt.Sprint(port)

	srv := &http.Server{Addr: "localhost:" + portStr, Handler: mux}
	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Warnf("Error starting test server: %v\n", err)
		}
	}()

	return srv, portStr
}

// Stop the test HTTP file server
func StopFileServer(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		fmt.Printf("Error shuting down test server: %v", err)
	}
}

// The single test we have so far
func TestTorrentDownloadFromLocalMachine(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(true, true)

	rootDir := "test_data/."
	tempRootDir := t.TempDir()

	// Copy input test dir to our temporary folder
	copy.Copy(rootDir, tempRootDir)

	srv, port := CreateAndStartFileServer(tempRootDir)
	defer StopFileServer(srv)

	destinationFolder := t.TempDir()

	// Create the 'server' torrent client, if that is a thing
	// This is the one that I expect to "serve the file", or seed ?
	findFreeListenPort := true
	client, err := MakeTorrentClient(destinationFolder, findFreeListenPort, true)
	if err != nil {
		t.Fatalf("cannot create seed torrent client: %s", err)
	}
	defer client.Close()

	// Build a url to the torrent file, which is not created yet
	torrentUrl := "http://localhost:" + port + "/a_test_file.txt.torrent"
	log.Printf("Torrent url: %s", torrentUrl)

	torrentPath := tempRootDir + "/a_test_file.txt.torrent"
	of, err := os.Create(torrentPath)
	if err != nil {
		t.Fatalf("cannot open torrent path for writing: %s", err)
	}
	defer of.Close()

	// Make a metainfo for the torrent file.
	mi := metainfo.MetaInfo{}
	mi.SetDefaults()

	// nothing is listening at this port at this point
	announceUrl := "udp://127.0.0.1:6969"
	// I tried using chihaya which is built on top of libtorrent/anacrolyx it seems
	// also tried to use the Local Port from the client for the seed.
	// I don't know if that 
	// announceUrl := fmt.Sprintf("udp://localhost:%d", client.LocalPort())
	log.Printf("Annouce url: %s", announceUrl)

	// Set an announce list.
	mi.AnnounceList = [][]string{
		{announceUrl},
	}
	mi.Announce = announceUrl

	info := metainfo.Info{
		PieceLength: 256 * 1024,
	}
	err = info.BuildFromFilePath(tempRootDir + "/a_test_file.txt")
	if err != nil {
		t.Fatal(err)
	}
	mi.InfoBytes, err = bencode.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}

	err = mi.Write(of)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("Torrent path: %s", torrentPath)

	// Tried 'adding the meta-info to the client, which I thought would start seeding it.
	// Seed is an option on the client-config, which I believe I tried to set to on.
	// _, err = client.AddTorrent(&mi)

	//
	// Announce the torrent / needed ?
	//
	if false {
		response, err := tracker.Announce{
			TrackerUrl: "udp://127.0.0.1:6969",
			Request: tracker.AnnounceRequest{
				InfoHash: mi.HashInfoBytes(),
				Port:     uint16(client.LocalPort()),
			},
		}.Do()
		if err != nil {
			t.Fatalf("cannot annouce torrent: %s", err)
		}
		log.Print(response)
	}

	// Finally download the file
	httpClient := &http.Client{}
	err = DownloadFileWithTorrent(destinationFolder, httpClient, torrentUrl)
	if err != nil {
		// We fail here at this point
		t.Fatalf("download should not fail, err = %s", err)
	}

	// List all downloaded files, our file should be there, but we don't make it this far
	files, err := filepath.Glob(destinationFolder + "/*")
	for _, file := range files {
		log.Printf("downloaded file -> %s", file)
	}
}
