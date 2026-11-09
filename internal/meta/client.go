package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/httpclient"
	"go.mau.fi/mautrix-meta/pkg/messagix/securehistory"
	"go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"lazyfmess/internal/session"
)

type EventHandler func(ctx context.Context, evt Event)

type Client struct {
	mx            *messagix.Client
	handler       EventHandler
	user          types.UserInfo
	initial       *table.LSTable
	dataDir       string
	devices       *sqlstore.Container
	device        *store.Device
	e2ee          *whatsmeow.Client
	newDevice     bool
	historyMu     sync.Mutex
	historyDevice *securehistory.Device
	mediaMu       sync.Mutex
	tempMedia     []string
	cookieUserID  int64
	platform      string
	cookieMu      sync.Mutex
	savedCookies  map[string]string
	savedNative   *session.NativeSession
	ready         chan struct{}
	connectErr    error
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
	cli := &Client{handler: handler, dataDir: dataDir, ready: make(chan struct{})}
	cli.cookieUserID, _ = strconv.ParseInt(sess.Values["c_user"], 10, 64)
	cli.platform = sess.Platform
	cli.savedCookies = sess.Values
	cli.savedNative = sess.NativeSession
	cli.mx = messagix.NewClient(mc, logger, &messagix.Config{})
	cli.mx.SetEventHandler(cli.onEvent)
	if sess.NativeSession != nil {
		native, err := toMessagixNative(sess.NativeSession)
		if err != nil {
			return nil, err
		}
		cli.mx.MessengerLite.SetNativeSession(native)
	}
	if err := cli.loadHistoryDevice(); err != nil {
		return nil, err
	}
	return cli, nil
}

func (c *Client) Connect(ctx context.Context) error {
	err := c.connect(ctx)
	if shouldRefreshCookies(err, c.mx.MessengerLite.GetNativeSession()) {
		if refreshErr := c.refreshCookies(ctx); refreshErr != nil {
			err = fmt.Errorf("%w; renewing the session from the saved login failed: %v", err, refreshErr)
		} else {
			err = c.connect(ctx)
		}
	}
	c.connectErr = err
	close(c.ready)
	if err == nil {
		c.persistCookies()
	}
	return err
}

func (c *Client) persistCookies() {
	c.cookieMu.Lock()
	defer c.cookieMu.Unlock()
	live := c.mx.GetCookies()
	if live == nil || !live.IsLoggedIn() {
		return
	}
	values := make(map[string]string)
	for name, value := range live.GetAll() {
		values[string(name)] = value
	}
	native := fromMessagixNative(c.mx.MessengerLite.GetNativeSession())
	if maps.Equal(values, c.savedCookies) && equalNative(native, c.savedNative) {
		return
	}
	if err := session.SaveCookies(c.dataDir, &session.Cookies{Platform: c.platform, Values: values, NativeSession: native}); err == nil {
		c.savedCookies = values
		c.savedNative = native
	}
}

func shouldRefreshCookies(connectErr error, native *types.NativeSession) bool {
	return errors.Is(connectErr, httpclient.ErrTokenInvalidated) && native != nil && native.AccessToken != ""
}

func (c *Client) refreshCookies(ctx context.Context) error {
	native := c.mx.MessengerLite.GetNativeSession()
	fresh, err := c.mx.MessengerLite.ExchangeTransientToken(ctx, native.AccessToken)
	if err != nil {
		return fmt.Errorf("exchange saved login token: %w", err)
	}
	if fresh.GetUserID() != c.cookieUserID && c.cookieUserID != 0 {
		return fmt.Errorf("exchange saved login token: account does not match")
	}
	c.mx.GetCookies().UpdateValues(fresh.GetAll())
	c.persistCookies()
	return nil
}

func toMessagixNative(s *session.NativeSession) (*types.NativeSession, error) {
	deviceID, err := uuid.Parse(s.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("parse native session device id: %w", err)
	}
	familyID, err := uuid.Parse(s.FamilyDeviceID)
	if err != nil {
		return nil, fmt.Errorf("parse native session family device id: %w", err)
	}
	return &types.NativeSession{AccessToken: s.AccessToken, AppID: s.AppID, DeviceID: deviceID, FamilyDeviceID: familyID}, nil
}

func fromMessagixNative(s *types.NativeSession) *session.NativeSession {
	if s == nil || s.AccessToken == "" {
		return nil
	}
	return &session.NativeSession{
		AccessToken:    s.AccessToken,
		AppID:          s.AppID,
		DeviceID:       s.DeviceID.String(),
		FamilyDeviceID: s.FamilyDeviceID.String(),
	}
}

func equalNative(a, b *session.NativeSession) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (c *Client) waitReady(ctx context.Context) error {
	select {
	case <-c.ready:
		if c.connectErr != nil {
			return fmt.Errorf("not connected: %w", c.connectErr)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) connect(ctx context.Context) error {
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
		u, initial, err := c.mx.LoadMessagesPage(ctx)
		switch {
		case err == nil:
			c.user = u
			c.initial = initial
		case errors.Is(err, httpclient.ErrTokenInvalidated):
			return fmt.Errorf("load messages page: %w", mapConnectError(err))
		}
	}

	if err := c.setupE2EE(ctx); err != nil {
		return err
	}
	if err := c.mx.Connect(ctx); err != nil {
		return fmt.Errorf("connect socket: %w", err)
	}
	c.e2ee.AddEventHandler(func(evt any) { c.onE2EEEvent(ctx, evt) })
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
	c.persistCookies()
	if c.e2ee != nil {
		c.e2ee.Disconnect()
	}
	if c.mx != nil {
		c.mx.Disconnect()
	}
	if c.devices != nil {
		_ = c.devices.Close()
	}
	c.cleanupTempMedia()
}

func (c *Client) AccountName() string {
	if c.user == nil {
		return ""
	}
	return c.user.GetName()
}

func (c *Client) AccountID() int64 {
	if c.cookieUserID != 0 {
		return c.cookieUserID
	}
	select {
	case <-c.ready:
	default:
		return 0
	}
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
	if native := c.mx.MessengerLite.GetNativeSession(); isNew && native != nil {
		device.FacebookUUID = native.DeviceID
	}
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
			c.persistCookies()
			state, err := c.mx.DumpState()
			if err != nil || len(state) == 0 {
				continue
			}
			_ = session.SaveState(c.dataDir, state)
		}
	}
}

func IsLoggedOut(err error) bool {
	return errors.Is(err, httpclient.ErrTokenInvalidated)
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
