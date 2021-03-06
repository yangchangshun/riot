// 一个微博 pinyin 搜索的例子。
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/go-ego/riot"
	"github.com/go-ego/riot/types"
)

const (
	// SecondsInADay seconds in a day
	SecondsInADay = 86400
	// MaxTokenProximity max token proximity
	MaxTokenProximity = 2
)

var (
	searcher      = riot.Engine{}
	wbs           = map[uint64]Weibo{}
	weiboData     = flag.String("weibo_data", "weibo.txt", "微博数据文件")
	dictFile      = flag.String("dict_file", "../../data/dict/dictionary.txt", "词典文件")
	stopTokenFile = flag.String("stop_token_file", "../../data/dict/stop_tokens.txt", "停用词文件")
	staticFolder  = flag.String("static_folder", "static", "静态文件目录")
)

// Weibo weibo json struct
type Weibo struct {
	Id           uint64 `json:"id"`
	Timestamp    uint64 `json:"timestamp"`
	UserName     string `json:"user_name"`
	RepostsCount uint64 `json:"reposts_count"`
	Text         string `json:"text"`
}

func indexWeibo() {
	fmt.Println("index start")

	file, err := os.Open(*weiboData)
	if err != nil {
		fmt.Println("read file error")
		return
	}
	defer file.Close()

	br := bufio.NewReader(file)

	var tokenDatas []types.TokenData
	var index uint64
	var tokens []string

	index = 1

	for {
		buf, _, c := br.ReadLine()
		if c == io.EOF {
			break
		}

		// fmt.Printf("buf: %s\n", buf)

		tokens = searcher.PinYin(string(buf))

		for i := 0; i < len(tokens); i++ {
			// fmt.Printf("tokens[%d]: %s\n", i, tokens[i])

			tokenData := types.TokenData{Text: tokens[i]}
			tokenDatas = append(tokenDatas, tokenData)
		}

		searcher.IndexDocument(index, types.DocIndexData{Tokens: tokenDatas, Fields: string(buf)}, false)
		index++
		searcher.IndexDocument(index, types.DocIndexData{Content: string(buf), Tokens: tokenDatas}, false)
		index++
	}

	searcher.FlushIndex()
	fmt.Println("index done")
}

/*******************************************************************************
    JSON-RPC
*******************************************************************************/

// JsonResponse json response
type JsonResponse struct {
	Docs []*Weibo `json:"docs"`
}

// JsonRpcServer json rpc server
func JsonRpcServer(w http.ResponseWriter, req *http.Request) {
	query := req.URL.Query().Get("query")
	fmt.Printf("Req: %s\n", query)
	output := searcher.Search(types.SearchRequest{
		Text: query,
		RankOptions: &types.RankOptions{
			OutputOffset: 0,
			MaxOutputs:   100,
		},
	})

	// fmt.Println("output...", output)

	// 整理为输出格式
	docs := []*Weibo{}
	for _, doc := range output.Docs {
		wb := wbs[doc.DocId]
		wb.Text = doc.Content
		docs = append(docs, &wb)
	}

	response, _ := json.Marshal(&JsonResponse{Docs: docs})
	// fmt.Println("response...", response)

	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, string(response))
}

/*******************************************************************************
	主函数
*******************************************************************************/
func main() {

	// runtime.GOMAXPROCS(runtime.NumCPU())

	// 解析命令行参数
	flag.Parse()

	// 初始化
	//gob.Register(WeiboScoringFields{})
	log.Print("searcher init start")
	searcher.Init(types.EngineInitOptions{
		Using:         4,
		SegmenterDict: *dictFile,
		StopTokenFile: *stopTokenFile,
		IndexerInitOptions: &types.IndexerInitOptions{
			//IndexType: types.LocationsIndex,
			IndexType: types.DocIdsIndex,
		},
		// 如果你希望使用持久存储，启用下面的选项
		// 默认使用leveldb持久化，如果你希望修改数据库类型
		// 请用 StorageEngine:"" 或者修改 RIOT_STORAGE_ENGINE 环境变量
		// UseStorage: true,
		// StorageFolder: "weibo_search",
		// StorageEngine: "bg",
	})
	log.Print("searcher init end")
	wbs = make(map[uint64]Weibo)

	// 索引
	go indexWeibo()

	// 捕获ctrl-c
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			log.Print("捕获Ctrl-c，退出服务器")
			searcher.Close()
			os.Exit(0)
		}
	}()

	http.HandleFunc("/json", JsonRpcServer)
	http.Handle("/", http.FileServer(http.Dir(*staticFolder)))
	log.Print("服务器启动")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
