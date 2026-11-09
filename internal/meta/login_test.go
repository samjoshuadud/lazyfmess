package meta

import (
	"os"
	"path/filepath"
	"testing"

	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	"maunium.net/go/mautrix/bridgev2"
)

func TestMapLoginStep(t *testing.T) {
	t.Parallel()
	l := &Login{dir: t.TempDir()}
	tests := []struct {
		name   string
		step   *bridgev2.LoginStep
		kind   LoginStepKind
		fields int
		secret bool
		image  bool
		cancel bool
	}{
		{"credentials", &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeUserInput, UserInputParams: &bridgev2.LoginUserInputParams{Fields: []bridgev2.LoginInputDataField{
			{ID: "username", Name: "Username", Type: bridgev2.LoginInputFieldTypeUsername},
			{ID: "password", Name: "Password", Type: bridgev2.LoginInputFieldTypePassword},
		}}}, LoginInput, 2, true, false, false},
		{"two factor code", &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeUserInput, Instructions: "Enter the code", UserInputParams: &bridgev2.LoginUserInputParams{CanCancel: true, Fields: []bridgev2.LoginInputDataField{
			{ID: "code", Name: "Code", Type: bridgev2.LoginInputFieldType2FACode},
		}}}, LoginInput, 1, false, false, true},
		{"captcha", &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeUserInput, UserInputParams: &bridgev2.LoginUserInputParams{
			Fields:      []bridgev2.LoginInputDataField{{ID: "captcha_code", Name: "Captcha code", Type: bridgev2.LoginInputFieldTypeCaptchaCode}},
			Attachments: []*bridgev2.LoginUserInputAttachment{{FileName: "captcha.png", Content: []byte("png"), Info: bridgev2.LoginUserInputAttachmentInfo{MimeType: "image/png"}}},
		}}, LoginInput, 1, false, true, false},
		{"approve elsewhere", &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeDisplayAndWait, Instructions: "Check your phone", DisplayAndWaitParams: &bridgev2.LoginDisplayAndWaitParams{CanCancel: true}}, LoginWait, 0, false, false, true},
		{"web check", &bridgev2.LoginStep{Type: bridgev2.LoginStepTypeCookies, Instructions: "Complete the reCAPTCHA"}, LoginUnsupported, 0, false, false, false},
	}
	for _, tt := range tests {
		got, err := l.mapStep(tt.step)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got.Kind != tt.kind || len(got.Fields) != tt.fields || got.CanCancel != tt.cancel {
			t.Errorf("%s: got %+v", tt.name, got)
		}
		if tt.secret != (len(got.Fields) > 1 && got.Fields[1].Secret) {
			t.Errorf("%s: password field secret flag wrong: %+v", tt.name, got.Fields)
		}
		if tt.image {
			if filepath.Dir(got.Attachment) != l.dir {
				t.Errorf("%s: attachment %q", tt.name, got.Attachment)
			} else if data, err := os.ReadFile(got.Attachment); err != nil || string(data) != "png" {
				t.Errorf("%s: attachment content %q %v", tt.name, data, err)
			}
		}
	}
	if err := l.OpenAttachment("/etc/passwd"); err == nil {
		t.Fatal("OpenAttachment must refuse files outside the login dir")
	}
}

func TestLoginFinishBuildsCredentials(t *testing.T) {
	t.Parallel()
	l := &Login{mx: nil, dir: t.TempDir()}
	empty := &cookies.Cookies{}
	if _, err := l.finish(empty); err == nil {
		t.Fatal("incomplete session must fail")
	}
}
