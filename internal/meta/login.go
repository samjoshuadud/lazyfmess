package meta

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/bloks"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
	"maunium.net/go/mautrix/bridgev2"

	"lazyfmess/internal/session"
)

type LoginStepKind int

const (
	LoginInput LoginStepKind = iota
	LoginWait
	LoginDone
	LoginUnsupported
)

type LoginField struct {
	ID          string
	Name        string
	Description string
	Secret      bool
	Options     []string
}

type LoginStep struct {
	Kind         LoginStepKind
	Instructions string
	Fields       []LoginField
	Attachment   string
	CanCancel    bool
	Credentials  *session.Cookies
}

type Login struct {
	mx  *messagix.Client
	dir string
}

var ErrLoginCheckpoint = errors.New("this account needs a security check: open Facebook in a browser or the official app, complete it, then try again")

func StartLogin(ctx context.Context, logWriter io.Writer) (*Login, LoginStep, error) {
	logger := zerolog.Nop()
	if logWriter != nil {
		logger = zerolog.New(logWriter).With().Timestamp().Str("component", "login").Logger()
	}
	mx := messagix.NewClient(&cookies.Cookies{Platform: types.MessengerLiteAndroid}, logger, &messagix.Config{})
	dir, err := os.MkdirTemp("", "lazyfmess-login-*")
	if err != nil {
		return nil, LoginStep{}, fmt.Errorf("create login temp dir: %w", err)
	}
	l := &Login{mx: mx, dir: dir}
	step, err := l.Submit(ctx, nil)
	if err != nil {
		l.Close()
		return nil, LoginStep{}, err
	}
	return l, step, nil
}

func (l *Login) Submit(ctx context.Context, input map[string]string) (LoginStep, error) {
	step, newCookies, err := l.mx.MessengerLite.DoLoginSteps(ctx, input)
	if err != nil {
		if errors.As(err, &bloks.CheckpointError{}) {
			return LoginStep{}, ErrLoginCheckpoint
		}
		return LoginStep{}, fmt.Errorf("log in: %w", err)
	}
	if step == nil {
		return l.finish(newCookies)
	}
	return l.mapStep(step)
}

func (l *Login) Wait(ctx context.Context) (LoginStep, error) {
	return l.Submit(ctx, nil)
}

func (l *Login) Cancel(ctx context.Context) (LoginStep, error) {
	if err := l.mx.MessengerLite.CancelLoginStep(ctx); err != nil {
		return LoginStep{}, fmt.Errorf("cancel login step: %w", err)
	}
	return l.Submit(ctx, nil)
}

func (l *Login) OpenAttachment(path string) error {
	if path == "" || filepath.Dir(path) != l.dir {
		return fmt.Errorf("open login image: unknown file")
	}
	if err := openLocalFile(path); err != nil {
		return fmt.Errorf("open login image: %w", err)
	}
	return nil
}

func (l *Login) Close() {
	_ = os.RemoveAll(l.dir)
}

func (l *Login) finish(newCookies *cookies.Cookies) (LoginStep, error) {
	if !newCookies.IsLoggedIn() {
		return LoginStep{}, fmt.Errorf("log in: Meta returned an incomplete session")
	}
	values := make(map[string]string)
	for name, value := range newCookies.GetAll() {
		values[string(name)] = value
	}
	return LoginStep{
		Kind: LoginDone,
		Credentials: &session.Cookies{
			Platform:      types.MessengerLiteAndroid.String(),
			Values:        values,
			NativeSession: fromMessagixNative(l.mx.MessengerLite.GetNativeSession()),
		},
	}, nil
}

func (l *Login) mapStep(step *bridgev2.LoginStep) (LoginStep, error) {
	out := LoginStep{Instructions: strings.TrimSpace(step.Instructions)}
	switch step.Type {
	case bridgev2.LoginStepTypeUserInput:
		out.Kind = LoginInput
		params := step.UserInputParams
		if params == nil {
			return out, nil
		}
		out.CanCancel = params.CanCancel
		for _, field := range params.Fields {
			out.Fields = append(out.Fields, LoginField{
				ID:          field.ID,
				Name:        field.Name,
				Description: field.Description,
				Secret:      field.Type == bridgev2.LoginInputFieldTypePassword,
				Options:     field.Options,
			})
		}
		path, err := l.saveImageAttachment(params.Attachments)
		if err != nil {
			return LoginStep{}, err
		}
		out.Attachment = path
	case bridgev2.LoginStepTypeDisplayAndWait:
		out.Kind = LoginWait
		out.CanCancel = step.DisplayAndWaitParams != nil && step.DisplayAndWaitParams.CanCancel
	default:
		out.Kind = LoginUnsupported
		out.Instructions = "This login needs a web page check (" + out.Instructions + "), which a terminal cannot show. Use browser cookies instead."
	}
	return out, nil
}

func (l *Login) saveImageAttachment(attachments []*bridgev2.LoginUserInputAttachment) (string, error) {
	for _, a := range attachments {
		if a == nil || !strings.HasPrefix(a.Info.MimeType, "image/") || len(a.Content) == 0 {
			continue
		}
		name := filepath.Base(a.FileName)
		if name == "." || name == string(filepath.Separator) {
			name = "captcha.png"
		}
		path := filepath.Join(l.dir, name)
		if err := os.WriteFile(path, a.Content, 0o600); err != nil {
			return "", fmt.Errorf("save login image: %w", err)
		}
		return path, nil
	}
	return "", nil
}
