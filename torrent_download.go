package datamover

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"runtime"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"

	log "github.com/sirupsen/logrus"
)

func GetFreePort() int {
	listener, err := net.Listen("tcp", ":0")
	defer listener.Close()
	if err != nil {
		return rand.Intn(40000) + 10000
	}

	port := listener.Addr().(*net.TCPAddr).Port
	return port
}

func MakeTorrentClient(destinationFolder string, findFreeListenPort bool, debugTorrent bool) (*torrent.Client, error) {
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DataDir = destinationFolder

	defaultListenAddr := "localhost" // on mac
	if runtime.GOOS == "linux" {
		defaultListenAddr = "[::]"
	}
	listenAddr := defaultListenAddr
	listenPort := 6881
	if findFreeListenPort {
		// Find a default free port as 6881 tends to be bound to which leads
		// to failure to start with:
		// subsequent listen: listen udp4 127.0.0.1:6881: bind: address already in use
		// This is required to use multiple client torrent concurrently too
		listenPort = GetFreePort()
	}
	clientConfig.SetListenAddr(fmt.Sprintf("%s:%d", listenAddr, listenPort))

	// Log all fields / This is useful when comparing behavior with a different torrent cli
	log.WithFields(log.Fields{"name": "DisableWebseeds", "value": clientConfig.DisableWebseeds}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisableTCP", "value": clientConfig.DisableTCP}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisableUTP", "value": clientConfig.DisableUTP}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisableIPv4", "value": clientConfig.DisableIPv4}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisableIPv6", "value": clientConfig.DisableIPv6}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisableAcceptRateLimiting", "value": clientConfig.DisableAcceptRateLimiting}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "NoDHT", "value": clientConfig.NoDHT}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "Debug", "value": clientConfig.Debug}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "Seed", "value": clientConfig.Seed}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "PublicIp4", "value": clientConfig.PublicIp4}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "PublicIp6", "value": clientConfig.PublicIp6}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisablePEX", "value": clientConfig.DisablePEX}).Info("torrent properties")
	log.WithFields(log.Fields{"name": "DisableWebtorrent", "value": clientConfig.DisableWebtorrent}).Info("torrent properties")

	return torrent.NewClient(clientConfig)
}

func DownloadFileWithTorrent(destinationFolder string, httpClient *http.Client, url string) error {
	// Create our torrent client
	// FIXME / the storage folder should be made unique, and deleted when we close the torrent
	findFreeListenPort := true
	client, err := MakeTorrentClient(destinationFolder, findFreeListenPort, false)
	if err != nil {
		return err
	}
	defer client.Close()

	log.Printf("fetching torrent from url: %s", url)

	// Fetch torrent
	response, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("Error downloading torrent file %s: %s", url, err)
	}

	if response.StatusCode != 200 {
		return errors.New("Non 200 response status code")
	}

	// Load the torrent data into our torrent client
	metaInfo, err := metainfo.Load(response.Body)
	if err != nil {
		return fmt.Errorf("Error loading torrent file %s: %s", url, err)
	}

	// Add the torrent for subsequent download
	torrent, err := client.AddTorrent(metaInfo)
	if err != nil {
		log.Errorf("Error adding torrent file %s: %s", url, err)
		return err
	}

	torrent.DownloadAll()

	if client.WaitAll() && torrent.Length() == torrent.BytesCompleted() {
		log.Print("downloaded all torrents")
	} else {
		return errors.New("The torrent files were not downloaded")
	}

	return nil
}
