package providers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

type WechatProvider struct {
	*ProviderData
}

func NewWechatProvider(p *ProviderData) *WechatProvider {
	p.ProviderName = "WeChat"
	if p.LoginURL == nil || p.LoginURL.String() == "" {
		p.LoginURL = &url.URL{
			Scheme: "https",
			Host:   "open.weixin.qq.com",
			Path:   "/connect/oauth2/authorize",
		}
	}
	if p.RedeemURL == nil || p.RedeemURL.String() == "" {
		p.RedeemURL = &url.URL{
			Scheme: "https",
			Host:   "api.weixin.qq.com",
			Path:   "/sns/oauth2/access_token",
		}
	}
	// ValidationURL is the API Base URL
	if p.ValidateURL == nil || p.ValidateURL.String() == "" {
		p.ValidateURL = &url.URL{
			Scheme: "https",
			Host:   "api.weixin.qq.com",
			Path:   "/sns/userinfo",
		}
	}
	if p.Scope == "" {
		p.Scope = "snsapi_userinfo"
	}
	return &WechatProvider{ProviderData: p}
}

func (p *WechatProvider) Redeem(redirectURL, code string) (s *SessionState, err error) {
	if code == "" {
		err = errors.New("missing code")
		return
	}

	params := url.Values{}
	params.Add("redirect_uri", redirectURL)
	params.Add("appid", p.ClientID)
	params.Add("secret", p.ClientSecret)
	params.Add("code", code)
	params.Add("grant_type", "authorization_code")
	if p.ProtectedResource != nil && p.ProtectedResource.String() != "" {
		params.Add("resource", p.ProtectedResource.String())
	}

	var req *http.Request
	req, err = http.NewRequest("POST", p.RedeemURL.String(), bytes.NewBufferString(params.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var resp *http.Response
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	var body []byte
	body, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("got %d from %q %s", resp.StatusCode, p.RedeemURL.String(), body)
		return
	}

	// blindly try json and x-www-form-urlencoded
	var jsonResponse struct {
		AccessToken string `json:"access_token"`
		OpenID      string `json:"openid"`
	}
	err = json.Unmarshal(body, &jsonResponse)
	if err == nil {
		s = &SessionState{
			AccessToken: jsonResponse.AccessToken,
			User:        jsonResponse.OpenID,
		}
		return
	}

	var v url.Values
	v, err = url.ParseQuery(string(body))
	if err != nil {
		return
	}
	if a := v.Get("access_token"); a != "" {
		openid := v.Get("openid")
		s = &SessionState{AccessToken: a, User: openid}
	} else {
		err = fmt.Errorf("no access token found %s", body)
	}
	return
}

func (p *WechatProvider) GetLoginURL(redirectURI, state string) string {
	var a url.URL
	a = *p.LoginURL
	params, _ := url.ParseQuery(a.RawQuery)
	params.Set("redirect_uri", redirectURI)
	if p.ApprovalPrompt == "qrcode" {
		a.Path = "/connect/qrconnect"
		params.Add("scope", "snsapi_login")
	} else {
		params.Add("scope", p.Scope)
	}
	params.Set("appid", p.ClientID)
	params.Set("response_type", "code")
	params.Add("state", state)
	a.RawQuery = params.Encode()
	return a.String()

}

func (p *WechatProvider) GetEmailAddress(s *SessionState) (string, error) {
	var user struct {
		Login    string `json:"openid"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	}

	endpoint := &url.URL{
		Scheme: p.ValidateURL.Scheme,
		Host:   p.ValidateURL.Host,
		Path:   p.ValidateURL.Path,
	}

	params, _ := url.ParseQuery(endpoint.RawQuery)
	params.Add("access_token", s.AccessToken)
	params.Add("openid", s.User)
	params.Add("lang", "zh_CN")
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not create new GET request: %v", err)
	}

	// req.Header.Set("Authorization", fmt.Sprintf("token %s", s.AccessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("got %d from %q %s",
			resp.StatusCode, endpoint.String(), body)
	}

	log.Printf("got %d from %q %s", resp.StatusCode, endpoint.String(), body)

	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("%s unmarshaling %s", err, body)
	}

	s.User = user.Nickname
	return user.Login, nil
}

func (p *WechatProvider) GetUserName(s *SessionState) (string, error) {
	var user struct {
		Login    string `json:"openid"`
		Nickname string `json:"nickname"`
		Email    string `json:"email"`
	}

	endpoint := &url.URL{
		Scheme: p.ValidateURL.Scheme,
		Host:   p.ValidateURL.Host,
		Path:   p.ValidateURL.Path,
	}

	params, _ := url.ParseQuery(endpoint.RawQuery)
	params.Set("access_token", s.AccessToken)
	params.Set("openid", s.Email)
	params.Set("lang", "zh_CN")
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequest("GET", endpoint.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not create new GET request: %v", err)
	}

	// req.Header.Set("Authorization", fmt.Sprintf("token %s", s.AccessToken))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("got %d from %q %s",
			resp.StatusCode, endpoint.String(), body)
	}

	log.Printf("got %d from %q %s", resp.StatusCode, endpoint.String(), body)

	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("%s unmarshaling %s", err, body)
	}

	return user.Nickname, nil
}
