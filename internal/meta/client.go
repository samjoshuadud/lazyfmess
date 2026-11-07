package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"fmesstui/internal/session"
)

type EventHandler func(ctx context.Context, evt Event)

type Event struct {
	Type  string
	Table *table.LSTable
	Err   error
}

type Client struct {
	mx        *messagix.Client
	handler   EventHandler
	user      types.UserInfo
	initial   *table.LSTable
	dataDir   string
	devices   *sqlstore.Container
	device    *store.Device
	e2ee      *whatsmeow.Client
	newDevice bool
}

func New(sess *session.Cookies, dataDir string, logWriter io.Writer, handler EventHandler) (*Client, error) {
	mc, err := toMessagixCookies(sess)
	if err != nil {
		return nil, err
	}
	logger := zerolog.Nop()
	if logWriter != nil {
		logger = zerolog.New(logWriter).With().Timestamp().Logger()
	}
	cli := &Client{handler: handler, dataDir: dataDir}
	cli.mx = messagix.NewClient(mc, logger, &messagix.Config{})
	cli.mx.SetEventHandler(cli.onEvent)
	return cli, nil
}

func (c *Client) Connect(ctx context.Context) error {
	if err := c.loadCachedState(); err != nil {
		user, initial, err := c.mx.LoadMessagesPage(ctx)
		if err != nil {
			return fmt.Errorf("load messages page: %w", mapConnectError(err))
		}
		c.user = user
		c.initial = initial
	} else {
		user, err := c.mx.GetCurrentAccount()
		if err != nil {
			return fmt.Errorf("load account from state: %w", err)
		}
		c.user = user
	}

	if err := c.setupE2EE(ctx); err != nil {
		return err
	}
	if err := c.mx.Connect(ctx); err != nil {
		return fmt.Errorf("connect socket: %w", err)
	}
	if err := c.e2ee.Connect(); err != nil {
		return fmt.Errorf("connect e2ee: %w", err)
	}
	go c.persistStateLoop(ctx)
	return nil
}

func (c *Client) Disconnect() {
	if c == nil {
		return
	}
	if state, err := c.mx.DumpState(); err == nil && len(state) > 0 {
		_ = session.SaveState(c.dataDir, state)
	}
	if c.e2ee != nil {
		c.e2ee.Disconnect()
	}
	if c.mx != nil {
		c.mx.Disconnect()
	}
	if c.devices != nil {
		_ = c.devices.Close()
	}
}

func (c *Client) AccountName() string {
	if c.user == nil {
		return ""
	}
	return c.user.GetName()
}

func (c *Client) AccountID() int64 {
	if c.user == nil {
		return 0
	}
	return c.user.GetFBID()
}

func (c *Client) InitialTable() *table.LSTable {
	return c.initial
}

func (c *Client) Messagix() *messagix.Client {
	return c.mx
}

func (c *Client) E2EE() *whatsmeow.Client {
	return c.e2ee
}

func (c *Client) loadCachedState() error {
	state, err := session.LoadState(c.dataDir)
	if err != nil {
		return err
	}
	if err := c.mx.LoadState(state); err != nil {
		if errors.Is(err, messagix.ErrCachedStateTooOld) {
			return err
		}
		return fmt.Errorf("load state: %w", err)
	}
	return nil
}

func (c *Client) setupE2EE(ctx context.Context) error {
	devices, err := session.OpenDeviceStore(ctx, c.dataDir)
	if err != nil {
		return err
	}
	c.devices = devices
	device, isNew, err := session.LoadOrCreateDevice(ctx, devices)
	if err != nil {
		return err
	}
	c.device = device
	c.newDevice = isNew
	c.mx.SetDevice(device)
	if isNew {
		fbid := c.AccountID()
		if fbid == 0 {
			return fmt.Errorf("register e2ee: account id is zero")
		}
		if err := c.mx.RegisterE2EE(ctx, fbid); err != nil {
			return fmt.Errorf("register e2ee: %w", err)
		}
		if err := device.Save(ctx); err != nil {
			return fmt.Errorf("save e2ee device: %w", err)
		}
	}
	if device.ID == nil {
		return fmt.Errorf("e2ee device jid is missing")
	}
	e2ee, err := c.mx.PrepareE2EEClient()
	if err != nil {
		return fmt.Errorf("prepare e2ee: %w", err)
	}
	c.e2ee = e2ee
	return nil
}

func (c *Client) persistStateLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state, err := c.mx.DumpState()
			if err != nil || len(state) == 0 {
				continue
			}
			_ = session.SaveState(c.dataDir, state)
		}
	}
}

func mapConnectError(err error) error {
	switch {
	case errors.Is(err, httpclient.ErrTokenInvalidated):
		return fmt.Errorf("cookies are expired or rejected by messenger; export a fresh cookies.json while logged in: %w", err)
	case errors.Is(err, httpclient.ErrCheckpointRequired):
		return fmt.Errorf("messenger requires a checkpoint in the browser before this client can connect: %w", err)
	case errors.Is(err, httpclient.ErrChallengeRequired):
		return fmt.Errorf("messenger requires a challenge in the browser before this client can connect: %w", err)
	default:
		return err
	}
}

func toMessagixCookies(sess *session.Cookies) (*cookies.Cookies, error) {
	platform := types.PlatformFromString(sess.Platform)
	if !platform.IsValid() || !platform.IsMessenger() {
		return nil, fmt.Errorf("unsupported platform %q", sess.Platform)
	}
	values := make(map[cookies.MetaCookieName]string, len(sess.Values))
	for k, v := range sess.Values {
		values[cookies.MetaCookieName(k)] = v
	}
	c := &cookies.Cookies{Platform: platform}
	c.UpdateValues(values)
	return c, nil
}

func (c *Client) onEvent(ctx context.Context, evt any) {
	if c.handler == nil {
		return
	}
	switch e := evt.(type) {
	case *table.LSTable:
		c.handler(ctx, Event{Type: "table", Table: e})
	case *messagix.ConnectedEvent:
		c.handler(ctx, Event{Type: "connected"})
	case *messagix.ReconnectedEvent:
		c.handler(ctx, Event{Type: "reconnected"})
	case *messagix.TransientDisconnectEvent:
		c.handler(ctx, Event{Type: "transient_disconnect", Err: e.Err})
	case *messagix.PermanentErrorEvent:
		c.handler(ctx, Event{Type: "permanent_error", Err: e.Err})
	default:
		c.handler(ctx, Event{Type: reflect.TypeOf(evt).String()})
	}
}
