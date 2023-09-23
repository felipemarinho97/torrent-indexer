package goscrape

/*
	This library partially implement BEP-0015.
	See http://www.bittorrent.org/beps/bep_0015.html
*/

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math/rand"
	"net"
	"net/url"
	"sync"
	"time"
)

const (
	pid            uint64 = 0x41727101980 // protocol id magic constant
	actionConnect  uint32 = 0
	actionAnnounce uint32 = 1
	actionScrap    uint32 = 2
	actionError    uint32 = 3
	defaultTimeout        = time.Second * 15
)

var (
	// ErrUnsupportedScheme is returned if the URL scheme is unsupported
	ErrUnsupportedScheme = errors.New("unsupported scrape scheme")
	// ErrTooManyInfohash is returned if more than 74 infohash are given
	ErrTooManyInfohash = errors.New("cannot lookup more than 74 infohash at once")
	// ErrRequest is returned if a write was not done completely
	ErrRequest = errors.New("udp packet was not entirely written")
	// ErrResponse is returned if the tracker sent an invalid response
	ErrResponse = errors.New("invalid response received from tracker")
	// ErrInvalidAction is returned if the tracker answered with an invalid action
	ErrInvalidAction = errors.New("invalid action")
	// ErrInvalidTransactionID is returned if the tracker answered with an invalid transaction id
	ErrInvalidTransactionID = errors.New("invalid transaction id received")
	// ErrRemote is returned if a remote error occured
	ErrRemote = errors.New("service unavailable")
	// ErrRetryLimit is returned when the maximum number of retries is exceeded
	ErrRetryLimit = errors.New("maximum number of retries exceeded")
)

// ScrapeResult represents one result returned by the Scrape method
type ScrapeResult struct {
	Infohash  []byte
	Seeders   uint32
	Leechers  uint32
	Completed uint32
}

// TorrentScrape represents the internal structure of goscrape
type Goscrape struct {
	sync.Mutex
	url          string
	conn         net.Conn
	connectionID uint64
	session      time.Time
	retries      int
	timeout      time.Duration
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

// New creates a new instance of goscrape for the given torrent tracker
func New(rawurl string) (*Goscrape, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}

	if u.Scheme != "udp" {
		return nil, ErrUnsupportedScheme
	}

	return &Goscrape{
		url:     u.Host,
		retries: 3,
		timeout: defaultTimeout,
	}, nil
}

// SetRetryLimit sets the maximum number of attempts to do before giving up
func (g *Goscrape) SetRetryLimit(retries int) {
	g.retries = retries
}

// SetTimeout configure the time to wait for a tracker to answer a query
func (g *Goscrape) SetTimeout(timeout time.Duration) {
	g.timeout = timeout
}

func (g *Goscrape) transactionID() uint32 {
	return uint32(rand.Int31())
}

func (g *Goscrape) connect() (net.Conn, uint64, error) {
	var err error

	g.Lock()
	defer g.Unlock()

	if time.Since(g.session) > time.Minute {
		// Get a new transaction ID
		tid := g.transactionID()

		// Prepare our outgoing UDP packet
		buf := make([]byte, 16)
		binary.BigEndian.PutUint64(buf[0:], pid)  // magic constant
		binary.BigEndian.PutUint32(buf[8:], 0)    // action connect
		binary.BigEndian.PutUint32(buf[12:], tid) // transaction id

		g.conn, err = net.DialTimeout("udp", g.url, g.timeout)
		if err != nil {
			return nil, 0, err
		}

		var n, retries int

		for {
			retries++

			// Set a write deadline
			g.conn.SetWriteDeadline(time.Now().Add(g.timeout))

			n, err = g.conn.Write(buf)
			if err != nil {
				return nil, 0, err
			}
			if n != len(buf) {
				return nil, 0, ErrRequest
			}

			// Set a read deadline
			g.conn.SetReadDeadline(time.Now().Add(g.timeout))

			// Reuse our buffer to read the response
			n, err = g.conn.Read(buf)
			if err, ok := err.(net.Error); ok && err.Timeout() {
				if retries > g.retries {
					return nil, 0, ErrRetryLimit
				}
				continue
			} else if err != nil {
				return nil, 0, err
			}
			break
		}

		if n != len(buf) {
			return nil, 0, ErrResponse
		}

		if action := binary.BigEndian.Uint32(buf[0:]); action != actionConnect {
			return nil, 0, ErrInvalidAction
		}
		if tid := binary.BigEndian.Uint32(buf[4:]); tid != tid {
			return nil, 0, ErrInvalidTransactionID
		}

		g.connectionID = binary.BigEndian.Uint64(buf[8:])
		g.session = time.Now()
	}
	return g.conn, g.connectionID, nil
}

// Scrape will scrape the given list of infohash and return a ScrapeResult struct
func (g *Goscrape) Scrape(infohash ...[]byte) ([]*ScrapeResult, error) {

	if len(infohash) > 74 {
		return nil, ErrTooManyInfohash
	}

	conn, connectionid, err := g.connect()
	if err != nil {
		return nil, err
	}

	// Get a new transaction ID
	tid := g.transactionID()

	// Prepare our outgoing UDP packet
	buf := make([]byte, 16+(len(infohash)*20))
	binary.BigEndian.PutUint64(buf[0:], connectionid) // connection id
	binary.BigEndian.PutUint32(buf[8:], 2)            // action scrape
	binary.BigEndian.PutUint32(buf[12:], tid)         // transaction id

	// Pack all the infohash together
	src := bytes.Join(infohash, []byte(""))

	// Create our temporary hex-decoded buffer
	dst := make([]byte, hex.DecodedLen(len(src)))

	_, err = hex.Decode(dst, src)
	if err != nil {
		return nil, err
	}

	// Copy the binary representation of the infohash
	// to the packet buffer
	copy(buf[16:], dst)

	response := make([]byte, 8+(12*len(infohash)))

	var n, retries int
	for {
		retries++

		// Set a write deadline
		conn.SetWriteDeadline(time.Now().Add(g.timeout))

		// Send the packet to the tracker
		n, err = conn.Write(buf)
		if err != nil {
			return nil, err
		}

		if n != len(buf) {
			return nil, ErrRequest
		}

		// Set a read deadline
		conn.SetReadDeadline(time.Now().Add(g.timeout))

		n, err = conn.Read(response)
		if err, ok := err.(net.Error); ok && err.Timeout() {
			if retries > g.retries {
				return nil, ErrRetryLimit
			}
			continue
		} else if err != nil {
			return nil, err
		}
		break
	}

	// Check expected packet size
	if n < 8+(12*len(infohash)) {
		return nil, ErrResponse
	}

	action := binary.BigEndian.Uint32(response[0:])

	if transactionid := binary.BigEndian.Uint32(response[4:]); transactionid != tid {
		return nil, ErrInvalidTransactionID
	}

	if action == actionError {
		return nil, ErrRemote
	}
	if action != actionScrap {
		return nil, ErrInvalidAction
	}

	r := make([]*ScrapeResult, len(infohash))

	offset := 8
	for i := 0; i < len(infohash); i++ {
		r[i] = &ScrapeResult{
			Infohash:  infohash[i],
			Seeders:   binary.BigEndian.Uint32(response[offset:]),
			Completed: binary.BigEndian.Uint32(response[offset+4:]),
			Leechers:  binary.BigEndian.Uint32(response[offset+8:]),
		}
		offset += 12
	}

	return r, nil
}
