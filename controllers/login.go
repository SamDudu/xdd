package controllers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/beego/beego/v2/core/logs"
	"github.com/SamDudu/xdd/models"

	"github.com/beego/beego/v2/client/httplib"
	qrcode "github.com/skip2/go-qrcode"
)

type LoginController struct {
	BaseController
}

type StepOne struct {
	SToken string `json:"s_token"`
}

type StepTwo struct {
	Token string `json:"token"`
}

type StepThree struct {
	CheckIP int    `json:"check_ip"`
	Errcode int    `json:"errcode"`
	Message string `json:"message"`
}

var JdCookieRunners sync.Map
var jdua = models.GetUserAgent

func (c *LoginController) GetQrcode() {
	if v := c.GetSession("jd_token"); v != nil {
		token := v.(string)
		if v, ok := JdCookieRunners.Load(token); ok {
			if len(v.([]interface{})) >= 2 {
				var url = `https://plogin.m.jd.com/cgi-bin/m/tmauth?appid=300&client_type=m&token=` + token
				data, _ := qrcode.Encode(url, qrcode.Medium, 256)
				c.Ctx.WriteString(`{"url":"` + url + `","img":"` + base64.StdEncoding.EncodeToString(data) + `"}`)
				return
			}
		}
	}
	var state = time.Now().Unix()
	var url = fmt.Sprintf(`https://plogin.m.jd.com/cgi-bin/mm/new_login_entrance?lang=chs&appid=300&returnurl=https://wq.jd.com/passport/LoginRedirect?state=%d&returnurl=https://home.m.jd.com/myJd/newhome.action?sceneval=2&ufc=&/myJd/home.action&source=wq_passport`,
		state)
	req := httplib.Get(url)
	req.Header("Connection", "Keep-Alive")
	req.Header("Content-Type", "application/x-www-form-urlencoded")
	req.Header("Accept", "application/json, text/plain, */*")
	req.Header("Accept-Language", "zh-cn")
	req.Header("Referer", url)
	req.Header("User-Agent", jdua())
	req.Header("Host", "plogin.m.jd.com")
	rsp, err := req.Response()
	if err != nil {
		c.Ctx.WriteString(err.Error())
		return
	}
	data, err := ioutil.ReadAll(rsp.Body)
	so := StepOne{}
	err = json.Unmarshal(data, &so)
	if err != nil {
		c.Ctx.WriteString(err.Error())
		return
	}
	cookies := strings.Join(rsp.Header.Values("Set-Cookie"), " ")
	var cookie = strings.Join([]string{
		"guid=" + FetchJdCookieValue("guid", cookies),
		"lang=chs",
		"lsid=" + FetchJdCookieValue("lsid", cookies),
		"lstoken=" + FetchJdCookieValue("lstoken", cookies),
	}, ";")
	state = time.Now().Unix()
	req = httplib.Post(
		fmt.Sprintf(`https://plogin.m.jd.com/cgi-bin/m/tmauthreflogurl?s_token=%s&v=%d&remember=true`,
			so.SToken,
			state),
	)
	req.Header("Connection", "Keep-Alive")
	req.Header("Content-Type", "application/x-www-form-urlencoded; Charset=UTF-8")
	req.Header("Accept", "application/json, text/plain, */*")
	req.Header("Cookie", cookie)
	req.Header("Referer", fmt.Sprintf(`https://plogin.m.jd.com/login/login?appid=300&returnurl=https://wqlogin2.jd.com/passport/LoginRedirect?state=%d&returnurl=//home.m.jd.com/myJd/newhome.action?sceneval=2&ufc=&/myJd/home.action&source=wq_passport`,
		state),
	)
	req.Header("User-Agent", jdua())
	req.Header("Host", "plogin.m.jd.com")
	req.Body(fmt.Sprintf(`{
		'lang': 'chs',
		'appid': 300,
		'returnurl': 'https://wqlogin2.jd.com/passport/LoginRedirect?state=%dreturnurl=//home.m.jd.com/myJd/newhome.action?sceneval=2&ufc=&/myJd/home.action&source=wq_passport',
	 }`, state))
	rsp, err = req.Response()
	if err != nil {
		c.Ctx.WriteString(err.Error())
		return
	}
	data, err = ioutil.ReadAll(rsp.Body)
	st := StepTwo{}
	err = json.Unmarshal(data, &st)
	if err != nil {
		c.Ctx.WriteString(err.Error())
		return
	}
	url = `https://plogin.m.jd.com/cgi-bin/m/tmauth?client_type=m&appid=300&token=` + st.Token
	cookies = strings.Join(rsp.Header.Values("Set-Cookie"), " ")
	okl_token := FetchJdCookieValue("okl_token", cookies)
	data, _ = qrcode.Encode(url, qrcode.Medium, 256)
	bot := c.GetString("tp")
	uid := c.GetQueryInt("uid")
	gid := c.GetQueryInt("gid")
	mid := c.GetQueryInt("mid")
	unm := c.GetString("unm")
	JdCookieRunners.Store(st.Token, []interface{}{cookie, okl_token, bot, uid, gid, mid, unm})
	if bot != "" {
		c.Ctx.ResponseWriter.Write(data)
		return
	}
	c.SetSession("jd_token", st.Token)
	c.SetSession("jd_cookie", cookie)
	c.SetSession("jd_okl_token", okl_token)
	c.Ctx.WriteString(`{"url":"` + url + `","img":"` + base64.StdEncoding.EncodeToString(data) + `"}`) //"data:image/png;base64," +
}

func init() {
	go func() {
		for {
			time.Sleep(time.Second)
			JdCookieRunners.Range(func(k, v interface{}) bool {
				jd_token := k.(string)
				vv := v.([]interface{})
				if len(vv) >= 2 {
					cookie := vv[0].(string)
					okl_token := vv[1].(string)
					bot := vv[2].(string)
					uid := vv[3].(int)
					gid := vv[4].(int)
					// fmt.Println(jd_token, cookie, okl_token)
					result, ck := CheckLogin(jd_token, cookie, okl_token)
					// fmt.Println(result)
					switch result {
					case "??????":
						switch bot {
						case "qq", "qqg":
							ck.Update(models.QQ, uid)
							if gid != 0 {
								go models.SendQQGroup(int64(gid), int64(uid), "????????????")
							} else {
								go models.SendQQ(int64(uid), "????????????")
							}
						case "tg", "tgg":
							ck.Update(models.Telegram, uid)
							if ck.Priority < 0 && models.GetEnv("AutoPriority") == models.True {
								ck.Update(models.Priority, -ck.Priority)
							}
							if gid != 0 {
								go models.SendTggMsg(int(gid), int(uid), "????????????", vv[5].(int), vv[6].(string))
							} else {
								go models.SendTgMsg(int(uid), "????????????")
							}
						}
					case "?????????????????????":
					case "":
					default: //??????
						switch bot {
						case "qq", "qqg":
							// ck.Update(models.QQ, uid)
							if gid != 0 {
								go models.SendQQGroup(int64(gid), int64(uid), "????????????")
							} else {
								go models.SendQQ(int64(uid), "????????????")
							}
						case "tg", "tgg":
							// ck.Update(models.Telegram, uid)
							if gid != 0 {
								go models.SendTggMsg(int(gid), int(uid), "????????????", vv[5].(int), vv[6].(string))
							} else {
								go models.SendTgMsg(int(uid), "????????????")
							}
						}
					}
				}
				return true
			})
		}
	}()
}

//Query ??????
func (c *LoginController) Query() {
	if v := c.GetSession("jd_token"); v == nil {
		c.Ctx.WriteString("?????????????????????")
		return
	} else {
		token := v.(string)
		if v, ok := JdCookieRunners.Load(token); !ok {
			c.Ctx.WriteString("?????????????????????")
			return
		} else {
			if len(v.([]interface{})) >= 2 {
				c.Ctx.WriteString("?????????????????????")
				return
			} else {
				pin := v.([]interface{})[0].(string)
				c.SetSession("pin", pin)
				if note := c.GetString("note"); note != "" {
					if ck, err := models.GetJdCookie(pin); err == nil {
						ck.Update(models.Note, note)
					}
				}
				// if strings.Contains(models.Config.Master, pin) {
				c.Ctx.WriteString("??????")
				// } else {
				// 	c.Ctx.WriteString("??????")
				// }
				return
			}
		}
	}
}

func CheckLogin(token, cookie, okl_token string) (string, *models.JdCookie) {
	state := time.Now().Unix()
	req := httplib.Post(
		fmt.Sprintf(`https://plogin.m.jd.com/cgi-bin/m/tmauthchecktoken?&token=%s&ou_state=0&okl_token=%s`,
			token,
			okl_token,
		),
	)
	req.Header("Referer", fmt.Sprintf(`https://plogin.m.jd.com/login/login?appid=300&returnurl=https://wqlogin2.jd.com/passport/LoginRedirect?state=%d&returnurl=//home.m.jd.com/myJd/newhome.action?sceneval=2&ufc=&/myJd/home.action&source=wq_passport`,
		state),
	)
	req.Header("Cookie", cookie)
	req.Header("Connection", "Keep-Alive")
	req.Header("Content-Type", "application/x-www-form-urlencoded; Charset=UTF-8")
	req.Header("Accept", "application/json, text/plain, */*")
	req.Header("User-Agent", jdua())
	req.Header("Host", "plogin.m.jd.com")

	req.Param("lang", "chs")
	req.Param("appid", "300")
	req.Param("returnurl", fmt.Sprintf("https://wqlogin2.jd.com/passport/LoginRedirect?state=%d&returnurl=//home.m.jd.com/myJd/newhome.action?sceneval=2&ufc=&/myJd/home.action", state))
	req.Param("source", "wq_passport")

	rsp, err := req.Response()
	if err != nil {
		return "", nil //err.Error()
	}
	data, err := ioutil.ReadAll(rsp.Body)
	sth := StepThree{}
	err = json.Unmarshal(data, &sth)
	if err != nil {
		return "", nil //err.Error()
	}
	switch sth.Errcode {
	case 0:
		cookies := strings.Join(rsp.Header.Values("Set-Cookie"), " ")
		pt_key := FetchJdCookieValue("pt_key", cookies)
		pt_pin := FetchJdCookieValue("pt_pin", cookies)
		if pt_pin == "" {
			JdCookieRunners.Delete(token)
			return sth.Message, nil
		}
		ck := models.JdCookie{
			PtKey: pt_key,
			PtPin: pt_pin,
			Hack:  models.False,
		}
		if nck, err := models.GetJdCookie(ck.PtPin); err == nil {
			nck.InPool(ck.PtKey)
			msg := fmt.Sprintf("???????????????%s", ck.PtPin)
			(&models.JdCookie{}).Push(msg)
			logs.Info(msg)
			if nck.Hack == models.True {
				ck.Update(models.Hack, models.False)
			}
		} else {
			models.NewJdCookie(&ck)
			msg := fmt.Sprintf("???????????????%s", ck.PtPin)
			(&models.JdCookie{}).Push(msg)
			logs.Info(msg)
		}
		go func() {
			models.Save <- &ck
		}()
		JdCookieRunners.Store(token, []interface{}{pt_pin})
		return "??????", &ck
	case 19: //Token????????????????????????
		JdCookieRunners.Delete(token)
		return sth.Message, nil
	case 21: //Token???????????????????????????
		JdCookieRunners.Delete(token)
		return sth.Message, nil
	case 176: //?????????????????????
		return sth.Message, nil
	case 258: //???????????????????????????
		return "", nil
	case 264: //???????????????????????????
		// JdCookieRunners.Delete(token)
		// return sth.Message, nil
	default:
		JdCookieRunners.Delete(token)
		// fmt.Println(sth)
	}
	return "", nil
}

func FetchJdCookieValue(key string, cookies string) string {
	match := regexp.MustCompile(key + `=([^;]*);{0,1}`).FindStringSubmatch(cookies)
	if len(match) == 2 {
		return match[1]
	} else {
		return ""
	}
}

func (c *LoginController) Cookie() {
	cookies := c.Ctx.Input.Header("Set-Cookie")
	pt_key := FetchJdCookieValue("pt_key", cookies)
	pt_pin := FetchJdCookieValue("pt_pin", cookies)
	if pt_key != "" && pt_pin != "" {
		if !models.HasPin(pt_pin) {
			models.NewJdCookie(&models.JdCookie{
				PtKey: pt_key,
				PtPin: pt_pin,
				Hack:  models.True,
			})
		} else if !models.HasKey(pt_key) {
			ck, _ := models.GetJdCookie(pt_pin)
			ck.InPool(pt_key)
		}
	}
}
