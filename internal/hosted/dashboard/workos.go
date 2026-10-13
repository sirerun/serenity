package dashboard

import (
	"net"
	"net/http"

	"github.com/sirerun/serenity/internal/hosted/browserflow"
)

const workOSStateCookie = "serenity_workos_state"

func (d *Dashboard) workOSCookieName() string {
	if d.Dev {
		return workOSStateCookie
	}
	return "__Host-" + workOSStateCookie
}

func (d *Dashboard) workOSStart(w http.ResponseWriter, r *http.Request) { d.beginWorkOS(w, r, "") }
func (d *Dashboard) workOSLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.session(w, r, true); !ok {
		return
	}
	cookie, err := r.Cookie("serenity_session")
	if err != nil {
		http.Error(w, "Sign in again", http.StatusUnauthorized)
		return
	}
	d.beginWorkOS(w, r, cookie.Value)
}
func (d *Dashboard) beginWorkOS(w http.ResponseWriter, r *http.Request, session string) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if peer := net.ParseIP(ip); peer != nil && peer.IsLoopback() {
		if forwarded := net.ParseIP(r.Header.Get("X-Serenity-Client-IP")); forwarded != nil {
			ip = forwarded.String()
		}
	}
	state, location, err := d.Identity.BeginWorkOS(r.Context(), session, ip)
	if err != nil {
		render(w, 503, view{Title: "Sign-in temporarily unavailable", Message: "Use your email link below, or try again shortly."})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: d.workOSCookieName(), Value: state, Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	if r.Method == http.MethodPost {
		browserflow.Continue(w, location)
		return
	}
	http.Redirect(w, r, location, http.StatusSeeOther)
}
func (d *Dashboard) workOSCallback(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(d.workOSCookieName())
	if err != nil {
		render(w, 400, view{Title: "Start a new sign-in", Message: "This sign-in attempt is no longer available. Use your email link below or return to sign-in."})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: d.workOSCookieName(), Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	session := ""
	if c, e := r.Cookie("serenity_session"); e == nil {
		session = c.Value
	}
	code := r.URL.Query().Get("code")
	if r.URL.Query().Get("error") != "" {
		code = ""
	}
	raw, err := d.Identity.CompleteWorkOS(r.Context(), r.URL.Query().Get("state"), cookie.Value, code, session)
	if err != nil {
		// Deliberately identical for unknown, unverified, disabled and existing
		// accounts. Provider error bodies, emails and tokens are never reflected.
		render(w, 400, view{Title: "Unable to finish sign-in", Message: "Use your Serenity email link below, then open Settings to connect another sign-in method. You can also return to sign-in and try again."})
		return
	}
	d.finishLogin(w, r, raw)
}
