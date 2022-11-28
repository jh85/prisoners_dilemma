package main

import (
	"fmt"
	"bufio"
	"net"
	"flag"
	"time"
	"math/rand"
	"encoding/json"
)

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

var (
	strategy string
)


func main() {
	rand.Seed(time.Now().UnixNano())

	addr := flag.String("addr", "localhost:19714", "server IP address:port")
	stg := flag.String("strategy", "RDM", "TFT,TTT,ALC,ALD,FDM,RDM,TLK,SLV,MST,GRO,JOS,DOW,GKP,MMC")
	flag.Parse()
	strategy = *stg

	conn,err := net.Dial("tcp", *addr)
	if err != nil {
		fmt.Println("dial error",conn,err)
		return
	}

	login_msg := make_login_msg(*stg,"password")

	cnt,err := conn.Write([]byte(login_msg))
	if err != nil {
		fmt.Println("write failed", cnt, err)
		return
	}

	b := bufio.NewReader(conn)
	for {
		line,err := b.ReadBytes(byte('\n'))
		if err != nil {
			fmt.Println("ReadBytes failed", line, err)
			break
		}
		switch msg2cmd(line) {
		case "SESSION":
			do_session(conn,line)
		case "RESULT":
			do_result(line)
		case "CONTINUE":
			do_continue(conn,line)
		default:
		}
	}
}

type MsgHead struct {
	Command string
}

func msg2cmd(line []byte) string {
	var m MsgHead
	if err := json.Unmarshal(line,&m); err != nil {
		fmt.Println("unknown message: ", line, err)
		return ""
	}
	return m.Command
}	

func do_continue(conn net.Conn, line []byte) {
	c := MsgContinue{Command:"CONTINUE",Action:"CONTINUE"}
	msg,_ := json.Marshal(c)
	msg = append(msg, '\n')
	cnt,err := conn.Write([]byte(msg))
	if err != nil {
		fmt.Println("write failed", cnt, err)
	}
	return
}

func do_result(line []byte) {
	var s MsgSession
	if err := json.Unmarshal(line,&s); err != nil {
		fmt.Println("unknown message: ", line, err)
		return
	}
	s.P0_his = []string{}
	s.P1_his = []string{}
	fmt.Println(s)
}

func TitForTat(s *MsgSession, ch chan string) {
	var action string
	if s.Round == 1 {
		action = "C"
	} else {
		var oppo_his []string
		if s.P0_id == "you" {
			oppo_his = s.P1_his
		} else {
			oppo_his = s.P0_his
		}
		action = oppo_his[s.Round-2]
	}
	ch <- action
}

func TitForTwoTat(s *MsgSession, ch chan string) {
	var action string
	if s.Round <= 2 {
		action = "C"
	} else {
		var oppo_his []string
		if s.P0_id == "you" {
			oppo_his = s.P1_his
		} else {
			oppo_his = s.P0_his
		}
		action = "C"
		if oppo_his[s.Round-2] == "D" &&
			oppo_his[s.Round-3] == "D" {
			action = "D"
		}
	}
	ch <- action
}

func all_C(ch chan string) {
	ch <- "C"
}

func all_D(ch chan string) {
	ch <- "D"
}

func random_choice(ch chan string) {
	switch rand.Intn(2) {
	case 0:
		ch <- "C"
	case 1:
		ch <- "D"
	}
}

func friedman(s *MsgSession, ch chan string) {
	var action string
	if s.Round == 1 {
		action = "C"
	} else {
		var oppo_his []string
		if s.P0_id == "you" {
			oppo_his = s.P1_his
		} else {
			oppo_his = s.P0_his
		}
		action = "C"
		for _,e := range oppo_his {
			if e == "D" {
				action = "D"
				break
			}
		}
	}
	ch <- action
}

func tullock(s *MsgSession, ch chan string) {
	var action string
	if s.Round <= 10 {
		action = "C"
	} else {
		var oppo_his []string
		if s.P0_id == "you" {
			oppo_his = s.P1_his
		} else {
			oppo_his = s.P0_his
		}
		count_C := 0
		for _,e := range oppo_his {
			if e == "C" {
				count_C += 1
			}
		}
		N := 100
		prob_C := int(N * count_C / len(oppo_his))
		if rand.Intn(N) < prob_C - 10 {
			action = "C"
		} else {
			action = "D"
		}		
	}
	ch <- action
}

func min(a,b int) int {
	if a < b {
		return a
	}
	return b
}

func master(s *MsgSession, ch chan string) {
	var action string
	if s.Round == 1 {
		action = "D"
	} else {
		var my_his []string
		var op_his []string
		if s.P0_id == "you" {
			my_his = s.P0_his
			op_his = s.P1_his
		} else {
			my_his = s.P1_his
			op_his = s.P0_his
		}
		oppo_is_slave := true
		for i := 0; i < min(s.Round-1,10); i++ {
			if my_his[i] != op_his[i] {
				oppo_is_slave = false
				break
			}
		}
		if oppo_is_slave {
			switch s.Round {
			case 1,3,5,7,9:
				action = "D"
			case 2,4,6,8,10:
				action = "C"
			default:
				action = "D"
			}
		} else {
			action = "D"
		}		
	}
	ch <- action
}

func slave(s *MsgSession, ch chan string) {
	var action string
	if s.Round == 1 {
		action = "D"
	} else {
		var my_his []string
		var op_his []string
		if s.P0_id == "you" {
			my_his = s.P0_his
			op_his = s.P1_his
		} else {
			my_his = s.P1_his
			op_his = s.P0_his
		}
		oppo_is_master := true
		for i := 0; i < min(s.Round-1,10); i++ {
			if my_his[i] != op_his[i] {
				oppo_is_master = false
				break
			}
		}
		if oppo_is_master {
			switch s.Round {
			case 1,3,5,7,9:
				action = "D"
			case 2,4,6,8,10:
				action = "C"
			default:
				action = "C"
			}
		} else {
			action = "D"
		}		
	}
	ch <- action
}

func true_with_rate(percentage int) bool {
	if percentage > rand.Intn(100) {
		return true
	} else {
		return false
	}
}

func grofman(s *MsgSession, ch chan string) {
	action := "C"
	if s.Round == 1 {
		action = "C"
	} else {
		var my_last string
		var op_last string
		if s.P0_id == "you" {
			my_last = s.P0_his[s.Round-2]
			op_last = s.P1_his[s.Round-2]
		} else {
			my_last = s.P1_his[s.Round-2]
			op_last = s.P0_his[s.Round-2]
		}
		if my_last != op_last && true_with_rate(28) {
			action = "C"
		} else {
			action = "D"
		}
	}
	ch <- action
}

func joss(s *MsgSession, ch chan string) {
	action := "C"
	if s.Round == 1 {
		action = "C"
	} else {
		var op_last string
		if s.P0_id == "you" {
			op_last = s.P1_his[s.Round-2]
		} else {
			op_last = s.P0_his[s.Round-2]
		}
		if op_last == "D" || true_with_rate(10) {
			action = "D"
		} else {
			action = "C"
		}
	}
	ch <- action
}

func downing(s *MsgSession, ch chan string) {
	var action string
	if s.Round <= 2 {
		action = "D"
	} else {
		var my_his []string
		var op_his []string
		if s.P0_id == "you" {
			my_his = s.P0_his
			op_his = s.P1_his
		} else {
			my_his = s.P1_his
			op_his = s.P0_his
		}

		cnt_CC := 0
		cnt_DC := 0
		for i := 0; i < s.Round-1; i++ {
			if my_his[i] == "C" && op_his[i] == "C" {
				cnt_CC += 1
			} else if my_his[i] == "D" && op_his[i] == "C" {
				cnt_DC += 1
			}
		}
	
		prob_CC := float64(cnt_CC) / float64(s.Round-1)
		prob_DC := float64(cnt_DC) / float64(s.Round-1)

		if prob_CC <= prob_DC {
			action = "D"
		} else {
			action = "C"
		}
	}
	ch <- action
}

func grasskamp(s *MsgSession, ch chan string) {
	var action string
	if s.Round <= 50 || (52 <= s.Round && s.Round <= 56) {
		ch2 := make(chan string)
		go TitForTat(s,ch2)
		action = <- ch2
	} else if s.Round == 51 {
		action = "D"
	} else {
		var op_his []string
		if s.P0_id == "you" {
			op_his = s.P1_his
		} else {
			op_his = s.P0_his
		}

		same := [56]string{
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"D","D","D","D","D","D"}
		rtft := [56]string{
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","C","C","C","C","C","C","C","C","C",
			"C","D","C","D","C","D"}
		
		op_56 := *(*[56]string)(op_his[:56])

		if op_56 == same || op_56 == rtft {
			// same or real TFT
			action = "C"
		} else {
			count := 0
			for i := 0; i < 56; i++ {
				if op_56[i] == "C" {
					count += 1
				}
			}
			if 20 < count && count < 36 {
				// random within conf interval 95%
				action = "D"
			} else {
				// something else
				if true_with_rate(33) {
					action = "D"
				} else {
					ch2 := make(chan string)
					go TitForTat(s,ch2)
					action = <- ch2
				}
			}
		}
	}
	ch <- action
}

func mimic(s *MsgSession, ch chan string) {
	var action string
	if s.Round <= 50 {
		go random_choice(ch)
	} else {
		var op_his []string
		if s.P0_id == "you" {
			op_his = s.P1_his
		} else {
			op_his = s.P0_his
		}

		cnt_C := 0
		for _,t := range op_his {
			if t == "C" {
				cnt_C += 1
			}
		}
	
		prob_C := float64(cnt_C) / float64(s.Round-1)

		if rand.Intn(1000) <= int(prob_C * 1000.0) {
			action = "C"
		} else {
			action = "D"
		}
		ch <- action
	}
}

func do_session(conn net.Conn, line []byte) {
	var s MsgSession
	if err := json.Unmarshal(line,&s); err != nil {
		fmt.Println("unknown message: ", line, err)
		return
	}
	msgid := s.Id
	ch := make(chan string)
	switch strategy {
	case "TFT":
		go TitForTat(&s, ch)
	case "TTT":
		go TitForTwoTat(&s, ch)
	case "ALC":
		go all_C(ch)
	case "ALD":
		go all_D(ch)
	case "FDM":
		go friedman(&s, ch)
	case "RDM":
		go random_choice(ch)
	case "TLK":
		go tullock(&s, ch)
	case "SLV":
		go slave(&s,ch)
	case "MST":
		go master(&s,ch)
	case "GRO":
		go grofman(&s,ch)
	case "JOS":
		go joss(&s,ch)
	case "DOW":
		go downing(&s,ch)
	case "GKP":
		go grasskamp(&s,ch)
	case "MMC":
		go mimic(&s,ch)
	default:
		fmt.Println("wrong strategy: ", strategy)
		// go random_choice(ch)
	}
		
	action := <- ch
	
	p := MsgPlay{Command:"PLAY",Id:msgid,Action:action}
	msg,_ := json.Marshal(p)
	msg = append(msg, '\n')
	cnt,err := conn.Write([]byte(msg))
	if err != nil {
		fmt.Println("write failed", cnt, err)
		return
	}
}

func make_login_msg(userid string, password string) []byte {
	m := MsgLogin{Command:"LOGIN",Userid:userid,Password:password}
	b,_ := json.Marshal(m)
	b = append(b,'\n')
	return b
}	

