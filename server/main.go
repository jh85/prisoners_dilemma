package main

import (
	"fmt"
	//	"bufio"
	// "runtime"
	"net"
	"os"
	"time"
	"flag"
	"math"
	"context"
	"errors"
	"sync"
	"bytes"
	"strconv"
	"math/rand"
	"encoding/json"
	"github.com/jinzhu/gorm"
        _ "github.com/jinzhu/gorm/dialects/sqlite"
	"golang.org/x/exp/maps"
)

var (
	dbDriver = ""
	dbName = "mmm.db"
)

type BufferTimeout struct {
        buf      []byte // contents are the bytes buf[off : len(buf)]
        off      int    // read at &buf[off], write at &buf[len(buf)]
        conn     net.Conn
}

func BTNewReader(conn net.Conn) *BufferTimeout {
        b := BufferTimeout{
                buf: make([]byte, 8192, 8192),
                off: 0,
                conn: conn,
        }
        return &b
}

//func (bt *BufferTimeout) ReadBytes(delim byte, timeout_sec int) (line []byte, err error) {
func (bt *BufferTimeout) ReadBytes(delim byte) (line []byte, err error) {
	timeout_msec := 0
	var t time.Time
	if timeout_msec != 0 {
		t = time.Now().Add(time.Duration(timeout_msec) * time.Millisecond)
	}
        for {
                i := bytes.IndexByte(bt.buf[:bt.off], delim)
                if i < 0 {
			if timeout_msec != 0 {
				bt.conn.SetReadDeadline(t)
			}
                        cnt,err := bt.conn.Read(bt.buf[bt.off:])
                        if err != nil {
                                fmt.Println("ReadBytes() error:", cnt, err)
                                return []byte{},err
                        }
                        bt.off += cnt
                } else {
                        line = make([]byte, i+1, i+1)
                        copy(line, bt.buf[:i+1])
                        copy(bt.buf, bt.buf[i+1:bt.off])
                        bt.off = bt.off - i - 1
                        return line,nil
                }
        }
}

type WaitQueue struct {
	readyq map[string]context.Context
	doneq map[string]context.Context
	scores map[string]int
	nSessions map[string]int
	mu sync.Mutex
}

func main() {
	addr := flag.String("addr", ":19714", "server IP address:port")
	usedb := flag.Bool("usedb", false, "set on to use database (sqlite3)")
	flag.Parse()
	if *usedb {
		dbDriver = "sqlite3"
	}
	ln,err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Println("listen error", ln, err)
		os.Exit(1)
	}

	if err := db_init(); err != nil {
		fmt.Println("db init error", err)
		ln.Close()
		os.Exit(1)
	}

	waitq := WaitQueue{
		readyq: make(map[string]context.Context),
		doneq: make(map[string]context.Context),
		scores: make(map[string]int),
		nSessions: make(map[string]int),
	}

	go func(wq *WaitQueue) {
		sessionid := 1000
		for {
			conn,err := ln.Accept()
			if err != nil {
				fmt.Println("accept error", conn, err)
				time.Sleep(time.Duration(10) * time.Second)
				continue
			}
			userid,err := check_login(conn)
			if err != nil {
				fmt.Println("failure login attempt from ",
					conn.RemoteAddr().String(), " err=", err)
				conn.Close()
				continue
			}
			fmt.Println("new client connected from ", conn.RemoteAddr().String())
			ctx := context.Background()
			ctx = context.WithValue(ctx, "conn", conn)
			ctx = context.WithValue(ctx, "userid", userid)
			user_session_id := userid + "-" + strconv.Itoa(sessionid)
			ctx = context.WithValue(ctx, "usid", user_session_id)
			ctx = context.WithValue(ctx, "addr", conn.RemoteAddr().String())
			ctx = context.WithValue(ctx, "rating", 1200.0)
			ctx = context.WithValue(ctx, "score", 0)
			ctx = context.WithValue(ctx, "session", 0)
			sessionid += 1

			wq.mu.Lock()
			wq.readyq[user_session_id] = ctx
			wq.mu.Unlock()
		}
	}(&waitq)

	for {
		var ctx0 context.Context = nil
		var ctx1 context.Context = nil
		waitq.mu.Lock()
		if len(waitq.readyq) > 1 {
			users := maps.Keys(waitq.readyq)
			tmp := rand.Perm(len(users))[:2]
			k0,k1 := users[tmp[0]],users[tmp[1]]
			ctx0 = waitq.readyq[k0]
			ctx1 = waitq.readyq[k1]
			delete(waitq.readyq, k0)
			delete(waitq.readyq, k1)
		} else if len(waitq.readyq) == 1 && len(waitq.doneq) > 0 {
			k0 := maps.Keys(waitq.readyq)[0]
			k1 := maps.Keys(waitq.doneq)[0]
			ctx0 = waitq.readyq[k0]
			ctx1 = waitq.doneq[k1]
			delete(waitq.readyq, k0)
			delete(waitq.doneq, k1)
		} else if len(waitq.doneq) > 1 {
			waitq.readyq,waitq.doneq = waitq.doneq,waitq.readyq
			users := maps.Keys(waitq.readyq)
			tmp := rand.Perm(len(users))[:2]
			k0,k1 := users[tmp[0]],users[tmp[1]]
			ctx0 = waitq.readyq[k0]
			ctx1 = waitq.readyq[k1]
			delete(waitq.readyq, k0)
			delete(waitq.readyq, k1)
		} else {
			waitq.readyq,waitq.doneq = waitq.doneq,waitq.readyq
		}
		waitq.mu.Unlock()
		if ctx0 == nil || ctx1 == nil {
			time.Sleep(time.Duration(700) * time.Millisecond)
		} else {
			go do_session(&waitq, ctx0, ctx1)
		}
	}
}

func ses2msg(s *Session, whom int, cmd string, msgid int) []byte {
	msg := MsgSession{
		Command: cmd,
		Id: msgid,
		Session: s.Session,
		StartTime: s.StartTime,
		EndTime: s.EndTime,
		Reward: s.Reward,
		Temptation: s.Temptation,
		Sucker: s.Sucker,
		Penalty: s.Penalty,
		Round: s.Round,
		Total: s.Total,
		P0_id: "",
		P1_id: "",
		P0_his: s.P0_his,
		P1_his: s.P1_his,
		P0_score: s.P0_score,
		P1_score: s.P1_score,
	}
	if whom == 0 {
		msg.P0_id = "you"
	} else {
		msg.P1_id = "you"
	}
	b,err := json.Marshal(msg)
	if err != nil {
		fmt.Println("Marshal failed")
	}
	b = append(b, '\n')
	return b
}

func make_session_id() string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	b := make([]rune,10)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func do_session(waitq *WaitQueue, ctx0 context.Context, ctx1 context.Context) {
	userid0 := ctx0.Value("userid").(string)
	usid0 := ctx0.Value("usid").(string)
	userid1 := ctx1.Value("userid").(string)
	usid1 := ctx1.Value("usid").(string)

	conn0 := ctx0.Value("conn").(net.Conn)
	conn1 := ctx1.Value("conn").(net.Conn)
	ctx0 = context.WithValue(ctx0, "b", BTNewReader(conn0))
	ctx1 = context.WithValue(ctx1, "b", BTNewReader(conn1))
	
	session_id := make_session_id()
	R_payoff := 7
	T_payoff := 10
	S_payoff := 0
	P_payoff := 4
	total_round := 1000
	
	session := Session{
		Session: session_id,
		StartTime: time.Now().UnixMilli(),
		EndTime: 0,
		Reward: R_payoff,
		Temptation: T_payoff,
		Sucker: S_payoff,
		Penalty: P_payoff,
		Round: 0,
		Total: total_round,
		P0_id: userid0,
		P1_id: userid1,
		P0_his: []string{},
		P1_his: []string{},
		P0_score: 0,
		P1_score: 0,
	}

	for i := 0; i < total_round; i++ {
		session.Round += 1
		ch0 := make(chan string)
		ch1 := make(chan string)

		go req_and_response(ctx0, 0, &session, ch0)
		go req_and_response(ctx1, 1, &session, ch1)

		p0act := <-ch0
		p1act := <-ch1

		p0score,p1score := calc_score(p0act, p1act, &session)

		session.P0_his = append(session.P0_his, p0act)
		session.P1_his = append(session.P1_his, p1act)
		session.P0_score += p0score
		session.P1_score += p1score

		if p0act == "E" || p1act == "E" || p0act == "T" || p1act == "T" {
			break
		}
	}

	session.EndTime = time.Now().UnixMilli()

	if false {
		r0 := ctx0.Value("rating").(float64)
		r1 := ctx1.Value("rating").(float64)
		k := 32.0
		w1 := 1.0 / (math.Pow(10.0, (r0-r1)/400.0) + 1.0)
		if session.P0_score >= session.P1_score {
			r0 += k * w1
			r1 -= k * w1
		} else {
			r0 -= k * (1.0-w1)
			r1 += k * (1.0-w1)
		}
		ctx0 = context.WithValue(ctx0, "rating", r0)
		ctx1 = context.WithValue(ctx1, "rating", r1)
	} else {
		sco0 := ctx0.Value("score").(int) + session.P0_score
		sco1 := ctx1.Value("score").(int) + session.P1_score
		ses0 := ctx0.Value("session").(int) + 1
		ses1 := ctx1.Value("session").(int) + 1
		r0 := float64(sco0) / float64(ses0)
		r1 := float64(sco1) / float64(ses1)
		ctx0 = context.WithValue(ctx0, "rating", r0)
		ctx1 = context.WithValue(ctx1, "rating", r1)
		ctx0 = context.WithValue(ctx0, "score", sco0)
		ctx1 = context.WithValue(ctx1, "score", sco1)
		ctx0 = context.WithValue(ctx0, "session", ses0)
		ctx1 = context.WithValue(ctx1, "session", ses1)
	}
	
	
	notified0 := make(chan struct {})
	notified1 := make(chan struct {})
	go send_result(ctx0, 0, &session, notified0)
	go send_result(ctx1, 1, &session, notified1)
	<-notified0
	<-notified1

	continue0 := make(chan bool)
	continue1 := make(chan bool)
	go check_continue(ctx0, continue0)
	go check_continue(ctx1, continue1)
	is_continue0 := <-continue0
	is_continue1 := <-continue1

	db_save_session(&session)

	var P0_score,P1_score,P0_sessions,P1_sessions int

	waitq.mu.Lock()
	if is_continue0 {
		waitq.doneq[usid0] = ctx0
	}
	if is_continue1 {
		waitq.doneq[usid1] = ctx1
	}
	waitq.scores[userid0] += session.P0_score
	waitq.nSessions[userid0] += 1
	waitq.scores[userid1] += session.P1_score
	waitq.nSessions[userid1] += 1
	P0_score = waitq.scores[userid0]
	P1_score = waitq.scores[userid1]
	P0_sessions = waitq.nSessions[userid0]
	P1_sessions = waitq.nSessions[userid1]
	waitq.mu.Unlock()

	if !is_continue0 {
		conn := ctx0.Value("conn").(net.Conn)
		conn.Close()
	}
	if !is_continue1 {
		conn := ctx1.Value("conn").(net.Conn)
		conn.Close()
	}
	P0_addr := ctx0.Value("addr").(string)
	P1_addr := ctx1.Value("addr").(string)
	P0_rating := ctx0.Value("rating").(float64)
	P1_rating := ctx1.Value("rating").(float64)
	print_result(&session, P0_score, P1_score, P0_sessions, P1_sessions, P0_addr, P1_addr, P0_rating, P1_rating)

	if !is_continue0 {
		fmt.Println("P0=",usid0, " is leaving")
	}
	if !is_continue1 {
		fmt.Println("P1=",usid1, " is leaving")
	}
}

func unix2datetime(unixtime int64) string {
	tm := time.Unix(unixtime / 1000, 0)
	return fmt.Sprintf("%v",tm)
}

func print_result(s *Session, p0_score int, p1_score int, p0_ses int, p1_ses int, p0_addr string, p1_addr string, p0_rating float64, p1_rating float64) {
	fmt.Print(s.Session, " ", unix2datetime(s.StartTime), " ", float64(s.EndTime - s.StartTime)/1000.0, " sec ", s.Round, "/",s.Total,
		" P0=", s.P0_id, "(",s.P0_score,") P1=",s.P1_id, "(",s.P1_score,")",
		" ", s.P0_id,"(",p0_addr,"):", p0_score,"/",p0_ses,"/",int(p0_rating),
		" ", s.P1_id,"(",p1_addr,"):", p1_score,"/",p1_ses,"/",int(p1_rating),"\n")

	//var m runtime.MemStats
	//runtime.ReadMemStats(&m)
	//fmt.Println(m)
	// fmt.Println(runtime.NumGoroutine())
}


func make_cont_msg() []byte {
	m := MsgContinue{Command: "CONTINUE",Action: ""}
	b,_ := json.Marshal(m)
	b = append(b, '\n')
	return b
}	

func check_continue(ctx context.Context, ch chan bool) {
	conn := ctx.Value("conn").(net.Conn)
	b := ctx.Value("b").(*BufferTimeout)
	cont_msg := make_cont_msg()
	cnt,err := conn.Write(cont_msg)
	if err != nil {
		fmt.Println("check_continue: Write failed", cnt, err)
		ch<-false
		return
	}
	// b := bufio.NewReader(conn)
	// b := BTNewReader(conn)
	line,err := b.ReadBytes(byte('\n'))
	if err != nil {
		fmt.Println("check_continue: ReadBytes failed", line, err)
		ch<-false
		return
	}
	var c MsgContinue
	if err := json.Unmarshal(line,&c); err != nil {
		fmt.Println("not a Continue", line, err)
		ch<-false
		return
	}
	if c.Action == "CONTINUE" {
		ch<-true
	} else {
		ch<-false
	}
	return
}

func calc_score(p0act string, p1act string, s *Session) (int,int) {
	if p0act == "E" || p1act == "E" {
		return s.Penalty,s.Penalty
	}
	if p0act == "T" || p1act == "T" {
		return s.Penalty,s.Penalty
	}
	if p0act == "C" {
		if p1act == "C" {
			return s.Reward,s.Reward
		} else {
			return s.Sucker,s.Temptation
		}
	} else {
		if p1act == "C" {
			return s.Temptation,s.Sucker
		} else {
			return s.Penalty,s.Penalty
		}
	}
}

func req_and_response(ctx context.Context, whom int, s *Session, ch chan string) {
	conn := ctx.Value("conn").(net.Conn)
	uid := ctx.Value("userid").(string)
	b := ctx.Value("b").(*BufferTimeout)

	//b :=bufio.NewReader(conn)
	// b := BTNewReader(conn)
	msgid := rand.Intn(math.MaxInt32)
	req := ses2msg(s, whom, "SESSION", msgid)
	cnt,err := conn.Write(req)
	if err != nil {
		fmt.Println("req_and_response: Write failed", cnt, err)
		ch<-"E"
		return
	}
	line,err := b.ReadBytes(byte('\n'))
	if err != nil {
		fmt.Println("req_and_response: ReadBytes failed", line, err, uid)
		if err,ok := err.(net.Error); ok && err.Timeout() {
			ch<-"T"
		} else {
			ch<-"E"
		}
		return
	}
	var p MsgPlay
	if err := json.Unmarshal(line,&p); err != nil {
		fmt.Println("Unmarshal failure:", line, err)
		ch<-"E"
		return
	}
	if p.Id != msgid {
		fmt.Println("message ID mismatch: response.ID=",
			p.Id, "request.ID=", msgid)
		ch<-"E"
		return
	}
	if p.Action == "C" || p.Action == "D" {
		ch<-p.Action
	} else {
		ch<-"E"
	}
	return
}

func send_result(ctx context.Context, whom int, s *Session, ch chan struct {}) {
	conn := ctx.Value("conn").(net.Conn)
	
	req := ses2msg(s, whom, "RESULT",0)
	cnt,err := conn.Write(req)
	if err != nil {
		fmt.Println("send_result: Write failed", cnt, err)
	}
	ch<-struct{}{}
}

func make_wait_message() []byte {
	m := MsgWait{Command: "WAIT"}
	b,_ := json.Marshal(m)
	b = append(b, '\n')
	return b
}

func check_login(conn net.Conn) (string,error) {
	// b := bufio.NewReader(conn)
	b := BTNewReader(conn)
	line,err := b.ReadBytes(byte('\n'))
	if err != nil {
		fmt.Println("check_login: ReadBytes failed", line, err)
		return "",err
	}
	var l MsgLogin
	if err := json.Unmarshal(line,&l); err != nil {
		fmt.Println("not a login: ", line, err)
		return "",err
	}
	if l.Command == "LOGIN" {
		userid,err := db_get_user(l.Userid, l.Password)
		if err != nil {
			return "",err
		}
		return userid,nil
	} else {
		return "",errors.New("wrong data")
	}
}

type MsgLogin struct {
        Command  string `json:"command"`
        Userid   string `json:"userid"`
        Password string `json:"password"`
}

type MsgPlay struct {
        Command string `json:"command"`
	Id      int    `json:"id"`
        Action  string `json:"action"`
}

type MsgContinue struct {
        Command string `json:"command"`
        Action  string `json:"action"`
}

type MsgWait struct {
	Command string `json:"command"`
}

type MsgSession struct {
	Command    string   `json:"command"`
	Id         int      `json:"id"`
        Session    string   `json:"session"`
	StartTime  int64    `json:"starttime"`
	EndTime    int64    `json:"endtime"`
        Reward     int      `json:"reward"`
        Temptation int      `json:"temptation"`
        Sucker     int      `json:"sucker"`
        Penalty    int      `json:"penalty"`
        Round      int      `json:"round"`
        Total      int      `json:"total"`
	P0_id      string   `json:"p0_id"`
	P1_id      string   `json:"p1_id"`
        P0_his     []string `json:"p0_his"`
	P1_his     []string `json:"p1_his"`
        P0_score   int      `json:"p0_score"`
	P1_score   int      `json:"p1_score"`
}

type Session struct {
        Session    string `gorm:"primary_key"`
	StartTime  int64
	EndTime    int64
        Reward     int
        Temptation int
        Sucker     int
        Penalty    int
        Round      int
        Total      int
	P0_id      string
	P1_id      string
        P0_his     []string
	P1_his     []string
        P0_score   int
	P1_score   int
}

type User struct {
        Userid string `gorm:"primary_key"`
        Password string
        Logintime string
        Address string
}

func db_init() error {
	if dbDriver == "" {
		return nil
	}
	db,err := gorm.Open(dbDriver,dbName)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer db.Close()
	db.AutoMigrate(&User{})
	db.AutoMigrate(&Session{})
	return nil
}
	
func db_save_session(s *Session) error {
	if dbDriver == "" {
		return nil
	}
	db,err := gorm.Open(dbDriver,dbName)
	if err != nil {
		fmt.Println(err)
		return err
	}
	defer db.Close()
	db.Create(s)
	return nil
}

func db_get_user(userid string, password string) (string,error) {
	if dbDriver == "" {
		return userid,nil
	}
	db,_ := gorm.Open(dbDriver, dbName)
	defer db.Close()
	var u User
	result := db.First(&u, "userid = ?", userid)
	if result.RowsAffected == 1 {
		if u.Password == password {
			return userid,nil
		} else {
			return "",errors.New("wrong password")
		}
	}
	return "",errors.New("no such user:" + userid)
}
