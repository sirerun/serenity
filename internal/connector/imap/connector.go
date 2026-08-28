// Package imap implements the IMAP connector (RFC 0001 §10.1), certified
// against Gmail with an app password (ADR 001). Cursors are UID-based
// (UIDVALIDITY + last-seen UID): Poll SELECTs the mailbox to read its
// current UIDVALIDITY, resets to UID 1 whenever that value has changed
// since the last run (RFC 3501 §2.3.1.1: a changed UIDVALIDITY means every
// previously assigned UID may now refer to something else, or nothing),
// then UID-SEARCHes and batch UID-FETCHes everything newer than the
// cursor. Messages the server has already expunged by fetch time are
// simply absent from the FETCH response (IMAP servers never error on a
// stale UID) -- Poll skips them and still advances the cursor past the
// UID it searched for, so a message deleted between SEARCH and FETCH can
// never wedge the cursor.
package imap

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"

	imapapi "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/domain"
)

// Gmail's certified IMAPS endpoint (ADR 001) and the connector's defaults.
const (
	GmailHost        = "imap.gmail.com"
	GmailPort        = 993
	DefaultMailbox   = "INBOX"
	defaultBatchSize = 200
)

// Connector implements connector.Connector for one IMAP mailbox. Dial
// opens an authenticated client connection; NewGmail supplies the
// production one (TLS to Gmail, app password from the OS keychain). Tests
// inject a Dial that talks to an in-process fake server instead.
type Connector struct {
	// Account is the mailbox address, also the IMAP username.
	Account string
	// Mailbox is the folder polled. Defaults to INBOX.
	Mailbox string
	// BatchSize bounds how many UIDs one FETCH command requests at a time
	// (RFC §10.1 pitfall: batch UID fetches). Defaults to 200.
	BatchSize int
	// Dial opens one authenticated IMAP connection for a single Poll call.
	// Required -- NewGmail supplies the production dialer.
	Dial func() (*imapclient.Client, error)
}

// cursorState is the JSON shape of a Connector's connector.Cursor.
type cursorState struct {
	UIDValidity uint32 `json:"uid_validity"`
	LastUID     uint32 `json:"last_uid"`
}

// NewGmail returns a Connector certified against Gmail's IMAPS endpoint
// (ADR 001), authenticating with the app password stored in the OS
// keychain for account (see StoredPassword; `serenity connectors auth
// imap` populates it).
func NewGmail(account string) *Connector {
	addr := net.JoinHostPort(GmailHost, strconv.Itoa(GmailPort))
	return &Connector{
		Account:   account,
		Mailbox:   DefaultMailbox,
		BatchSize: defaultBatchSize,
		Dial: func() (*imapclient.Client, error) {
			password, err := StoredPassword(account)
			if err != nil {
				return nil, fmt.Errorf("imap: app password for %s: %w (run `serenity connectors auth imap`?)", account, err)
			}
			return dialAndLogin(func() (*imapclient.Client, error) {
				return imapclient.DialTLS(addr, &imapclient.Options{})
			}, account, password)
		},
	}
}

// dialAndLogin runs dial then logs in, closing the connection on any
// login failure so a bad password never leaks an open socket.
func dialAndLogin(dial func() (*imapclient.Client, error), account, password string) (*imapclient.Client, error) {
	client, err := dial()
	if err != nil {
		return nil, fmt.Errorf("imap: connect: %w", err)
	}
	if err := client.Login(account, password).Wait(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("imap: login %s: %w", account, err)
	}
	return client, nil
}

// Name identifies this connector's runs in the jobs table -- one row per
// account, since a person may connect more than one mailbox.
func (c *Connector) Name() string { return "imap:" + c.Account }

func (c *Connector) mailbox() string {
	if c.Mailbox == "" {
		return DefaultMailbox
	}
	return c.Mailbox
}

func (c *Connector) batchSize() int {
	if c.BatchSize <= 0 {
		return defaultBatchSize
	}
	return c.BatchSize
}

// Poll fetches every message newer than cursor from the mailbox. See the
// package doc for the UIDVALIDITY-reset and mid-fetch-expunge handling.
func (c *Connector) Poll(ctx context.Context, cursor connector.Cursor) ([]connector.RawItem, connector.Cursor, error) {
	state, err := decodeCursor(cursor)
	if err != nil {
		return nil, cursor, fmt.Errorf("imap: decode cursor: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, cursor, err
	}

	client, err := c.Dial()
	if err != nil {
		return nil, cursor, err
	}
	defer func() {
		_ = client.Logout().Wait()
		_ = client.Close()
	}()

	mailbox := c.mailbox()
	selectData, err := client.Select(mailbox, nil).Wait()
	if err != nil {
		return nil, cursor, fmt.Errorf("imap: select %s: %w", mailbox, err)
	}

	if state.UIDValidity != 0 && state.UIDValidity != selectData.UIDValidity {
		// UIDVALIDITY changed: every UID this cursor remembers may now name
		// a different message, or none. Discard it and start from UID 1 --
		// content-addressed dedup downstream (internal/store.SourceStore)
		// keeps a full re-poll from creating duplicate sources.
		state = cursorState{}
	}
	state.UIDValidity = selectData.UIDValidity

	if err := ctx.Err(); err != nil {
		return nil, cursor, err
	}

	// UIDNext is one past the highest UID the server has ever assigned, so
	// state.LastUID+1 >= UIDNext proves there is nothing new without a
	// SEARCH round trip. This also sidesteps an IMAP range ambiguity: some
	// servers resolve "n:*" for n beyond the current max by swapping the
	// bounds instead of returning empty, which would silently re-return
	// the already-seen top message forever once a mailbox goes quiet.
	if selectData.UIDNext != 0 && uint32(selectData.UIDNext) <= state.LastUID+1 {
		return nil, encodeCursor(state), nil
	}

	var wanted imapapi.UIDSet
	wanted.AddRange(imapapi.UID(state.LastUID+1), 0) // 0 == "*": everything from state.LastUID+1 onward
	searchData, err := client.UIDSearch(&imapapi.SearchCriteria{UID: []imapapi.UIDSet{wanted}}, nil).Wait()
	if err != nil {
		return nil, cursor, fmt.Errorf("imap: search %s: %w", mailbox, err)
	}

	uids := searchData.AllUIDs()
	if len(uids) == 0 {
		return nil, encodeCursor(state), nil
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })

	items := make([]connector.RawItem, 0, len(uids))
	batch := c.batchSize()
	for start := 0; start < len(uids); start += batch {
		if err := ctx.Err(); err != nil {
			return items, cursor, err
		}
		end := start + batch
		if end > len(uids) {
			end = len(uids)
		}

		var chunk imapapi.UIDSet
		chunk.AddNum(uids[start:end]...)
		fetched, err := client.Fetch(chunk, &imapapi.FetchOptions{
			UID:          true,
			InternalDate: true,
			BodySection:  []*imapapi.FetchItemBodySection{{}},
		}).Collect()
		if err != nil {
			return items, cursor, fmt.Errorf("imap: fetch %s: %w", mailbox, err)
		}

		for _, msg := range fetched {
			body := msg.FindBodySection(&imapapi.FetchItemBodySection{})
			if body == nil {
				// Expunged between SEARCH and this FETCH batch -- the
				// server silently omits it. Skip; the cursor still
				// advances past its UID below.
				continue
			}
			uid := uint32(msg.UID)
			items = append(items, connector.RawItem{
				URI:        fmt.Sprintf("imap://%s/%s/%d", c.Account, mailbox, uid),
				Kind:       "email",
				Bytes:      body,
				OccurredAt: msg.InternalDate,
				Meta: map[string]string{
					"uid":         strconv.FormatUint(uint64(uid), 10),
					"uidvalidity": strconv.FormatUint(uint64(state.UIDValidity), 10),
					"mailbox":     mailbox,
					"account":     c.Account,
				},
			})
		}
	}

	// Advance past every UID this poll searched for, even ones a
	// concurrent expunge removed before the FETCH reached them -- a
	// message that vanished is not a message to retry.
	state.LastUID = uint32(uids[len(uids)-1])
	return items, encodeCursor(state), nil
}

// ToSource converts one fetched message into the domain.Source the store's
// content-address dedup runs on. SHA256 is left unset -- SourceStore.Write
// computes it from Bytes, never trusting a caller-supplied digest.
func (c *Connector) ToSource(item connector.RawItem) (domain.Source, error) {
	return domain.Source{
		Kind:       item.Kind,
		URI:        item.URI,
		OccurredAt: item.OccurredAt,
		Meta:       item.Meta,
	}, nil
}

func decodeCursor(c connector.Cursor) (cursorState, error) {
	if len(c) == 0 {
		return cursorState{}, nil
	}
	var s cursorState
	if err := json.Unmarshal(c, &s); err != nil {
		return cursorState{}, err
	}
	return s, nil
}

func encodeCursor(s cursorState) connector.Cursor {
	b, err := json.Marshal(s)
	if err != nil {
		// cursorState is two uint32 fields -- Marshal cannot fail.
		panic(fmt.Sprintf("imap: marshal cursor: %v", err))
	}
	return connector.Cursor(b)
}
