package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"
)

//go:embed static/*
var staticFiles embed.FS

var mainClient *tdx.Client
var exClient *tdx.Client

type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func main() {
	var err error
	mainClient, err = tdx.DialDefault()
	if err != nil {
		log.Printf("⚠️ A股连接失败: %v", err)
	}
	log.Println("✅ A股行情已连接")

	go func() {
		for i := 0; i < 3; i++ {
			exClient, err = tdx.DialExHqDefault()
			if err == nil {
				log.Println("✅ 美股/港股行情已连接")
				return
			}
			time.Sleep(2 * time.Second)
		}
		log.Printf("⚠️ 美股/港股连接失败: %v", err)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleWebUI)
	mux.HandleFunc("/api/quote", handleQuote)
	mux.HandleFunc("/api/kline", handleKline)
	mux.HandleFunc("/api/finance", handleFinance)
	mux.HandleFunc("/api/f10", handleF10)
	mux.HandleFunc("/api/us/quote", handleUSQuote)
	mux.HandleFunc("/api/hk/quote", handleHKQuote)
	mux.HandleFunc("/api/health", handleHealth)

	addr := ":8080"
	log.Printf("🚀 TDX API Server 启动于 %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func jsonResp(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(APIResponse{0, "success", data})
}

func jsonErr(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(400)
	json.NewEncoder(w).Encode(APIResponse{-1, msg, nil})
}

func cli() *tdx.Client {
	if mainClient == nil {
		mainClient, _ = tdx.DialDefault()
	}
	return mainClient
}

func handleWebUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl, _ := template.ParseFS(staticFiles, "static/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, nil)
}

func handleQuote(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" { jsonErr(w, "缺少 code"); return }
	c := cli()
	if c == nil { jsonErr(w, "未连接"); return }
	q, err := c.GetQuote(code)
	if err != nil || len(q) == 0 { jsonErr(w, "无数据"); return }
	qt := q[0]
	k := qt.Kline
	jsonResp(w, map[string]interface{}{
		"code": qt.Code, "price": k.Close.Float64(), "lastClose": k.Last.Float64(),
		"open": k.Open.Float64(), "high": k.High.Float64(), "low": k.Low.Float64(),
		"volume": k.Volume, "amount": k.Amount.Float64(), "time": qt.ServerTime,
	})
}

func handleKline(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" { jsonErr(w, "缺少 code"); return }
	cnt := uint16(10)
	if s := r.URL.Query().Get("count"); s != "" {
		if n, e := strconv.ParseUint(s, 10, 16); e == nil { cnt = uint16(n) }
	}
	c := cli()
	if c == nil { jsonErr(w, "未连接"); return }
	bars, err := c.GetKlineDay(code, 0, cnt)
	if err != nil || bars == nil || len(bars.List) == 0 { jsonErr(w, "无数据"); return }
	type Bar struct {
		Date string `json:"date"`; Open float64 `json:"open"`; High float64 `json:"high"`
		Low float64 `json:"low"`; Close float64 `json:"close"`; Volume int64 `json:"volume"`
	}
	list := make([]Bar, len(bars.List))
	for i, k := range bars.List {
		list[i] = Bar{k.Time.Format("2006-01-02"), k.Open.Float64(), k.High.Float64(), k.Low.Float64(), k.Close.Float64(), k.Volume}
	}
	jsonResp(w, map[string]interface{}{"code": code, "count": len(list), "list": list})
}

func handleFinance(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" { jsonErr(w, "缺少 code"); return }
	mkt := mktEnum(r.URL.Query().Get("mkt"))
	c := cli()
	if c == nil { jsonErr(w, "未连接"); return }
	f, err := c.GetFinanceInfo(mkt, code)
	if err != nil || f == nil { jsonErr(w, "无数据"); return }
	jsonResp(w, map[string]interface{}{
		"code": code, "updatedDate": f.UpdatedDate, "ipoDate": f.IPODate,
		"liuTongGuBen": f.LiuTongGuBen, "zongGuBen": f.ZongGuBen,
		"zongZiChan": f.ZongZiChan, "jingZiChan": f.JingZiChan,
		"zhuYingShouRu": f.ZhuYingShouRu, "zhuYingLiRun": f.ZhuYingLiRun,
		"yingYeLiRun": f.YingYeLiRun, "jingLiRun": f.JingLiRun,
		"jingYingXianJinLiu": f.JingYingXianJinLiu, "guDongRenShu": f.GuDongRenShu,
		"cunHuo": f.CunHuo,
	})
}

func handleF10(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" { jsonErr(w, "缺少 code"); return }
	mkt := mktEnum(r.URL.Query().Get("mkt"))
	catIdx := -1
	if s := r.URL.Query().Get("cat"); s != "" {
		catIdx, _ = strconv.Atoi(s)
	}
	c := cli()
	if c == nil { jsonErr(w, "未连接"); return }
	cats, err := c.GetCompanyCategory(mkt, code)
	if err != nil { jsonErr(w, err.Error()); return }
	type Cat struct {
		Index int `json:"index"`; Name string `json:"name"`
		Filename string `json:"filename,omitempty"`; Length int `json:"length"`
	}
	list := make([]Cat, len(cats))
	for i, c := range cats {
		list[i] = Cat{i, c.Name, c.Filename, int(c.Length)}
	}
	if catIdx >= 0 && catIdx < len(cats) {
		ct, err := c.GetCompanyContent(mkt, code, cats[catIdx].Filename, cats[catIdx].Start, cats[catIdx].Length)
		if err != nil { jsonErr(w, err.Error()); return }
		jsonResp(w, map[string]interface{}{"categories": list, "content": ct})
		return
	}
	jsonResp(w, map[string]interface{}{"categories": list})
}

func handleUSQuote(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" { jsonErr(w, "缺少 code"); return }
	ex := getEx()
	if ex == nil { jsonErr(w, "美股未连接"); return }
	q, err := ex.ExQuote(74, code)
	if err != nil || q == nil || q.Price == 0 { jsonErr(w, "无数据"); return }
	jsonResp(w, map[string]interface{}{
		"code": q.Code, "price": q.Price, "preClose": q.PreClose,
		"open": q.Open, "high": q.High, "low": q.Low, "volume": q.ZongLiang,
	})
}

func handleHKQuote(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" { jsonErr(w, "缺少 code"); return }
	ex := getEx()
	if ex == nil { jsonErr(w, "港股未连接"); return }
	q, err := ex.ExQuote(31, code)
	if err != nil || q == nil || q.Price == 0 { jsonErr(w, "无数据"); return }
	jsonResp(w, map[string]interface{}{
		"code": q.Code, "price": q.Price, "preClose": q.PreClose,
		"open": q.Open, "high": q.High, "low": q.Low, "volume": q.ZongLiang,
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	st := "ok"
	if mainClient == nil { st = "disconnected" }
	exSt := "disconnected"
	if exClient != nil { exSt = "ok" }
	jsonResp(w, map[string]string{
		"status": st, "ex_status": exSt,
		"version": "1.0.0", "server_time": time.Now().Format("2006-01-02 15:04:05"),
	})
}

func parseMkt(s string) uint8 {
	if s == "0" || s == "sz" { return 0 }
	return 1
}

func mktEnum(s string) protocol.Exchange {
	if s == "0" || s == "sz" { return protocol.ExchangeSZ }
	return protocol.ExchangeSH
}

func getEx() *tdx.Client {
	if exClient == nil {
		exClient, _ = tdx.DialExHqDefault()
	}
	return exClient
}
