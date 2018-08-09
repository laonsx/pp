package rpc

import (
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"gamelib/codec"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	callTimeout  = 5 * time.Second
	client       *Client
	streamClient = make(map[string]Game_StreamClient)
)

// Client rpc Client结构
type Client struct {
	name           string
	mux            sync.Mutex
	clients        map[string]GameClient
	cluster        map[string]string
	servicesMap    map[string]uint16
	servicesNumMap map[uint16]string
	opts           []grpc.DialOption
}

// InitClient 初始化客户端
func InitClient(name string, cluster map[string]string, services [][]string, opts []grpc.DialOption) {

	client = new(Client)
	client.clients = make(map[string]GameClient)
	client.cluster = cluster
	client.name = name

	serviceMap := make(map[string]uint16)
	servicesNumMap := make(map[uint16]string)

	for _, v := range services {

		pnum, _ := strconv.Atoi(v[0])
		sname := v[1]
		node := v[2]

		serviceMap[sname] = uint16(pnum)
		servicesNumMap[uint16(pnum)] = node + "|" + sname
	}
	client.servicesMap = serviceMap
	client.servicesNumMap = servicesNumMap
	client.opts = opts
}

func ReloadMethodConf(services [][]string) {

	client.mux.Lock()
	defer client.mux.Unlock()

	serviceMap := make(map[string]uint16)
	servicesNumMap := make(map[uint16]string)

	for _, v := range services {

		pnum, _ := strconv.Atoi(v[0])
		sname := v[1]
		node := v[2]

		serviceMap[sname] = uint16(pnum)
		servicesNumMap[uint16(pnum)] = node + "|" + sname
	}

	client.servicesMap = serviceMap
	client.servicesNumMap = servicesNumMap
}

func (c *Client) newClient(node string) (GameClient, error) {

	if v, ok := c.clients[node]; ok {

		return v, nil
	}

	c.mux.Lock()
	defer c.mux.Unlock()

	if addr, ok := c.cluster[node]; ok {

		conn, err := grpc.Dial(addr, c.opts...)
		if err != nil {

			return nil, err
		}

		gameClient := NewGameClient(conn)
		c.clients[node] = gameClient

		return gameClient, nil
	}

	return nil, errors.New("node conf")
}

// GetName 根据协议号 获取节点名称和服务名
func GetName(pnum uint16) (nname string, sname string, err error) {

	if s, ok := client.servicesNumMap[pnum]; ok {

		arr := strings.Split(s, "|")
		nname = arr[0]
		sname = arr[1]
	} else {

		err = errors.New("service not found")
	}

	return
}

// GetPNum 根据服务名 获取协议号
func GetPNum(service string) (pnum uint16, err error) {

	if n, ok := client.servicesMap[service]; ok {

		pnum = n
	} else {

		err = errors.New("pnum not found")
	}

	return
}

// Stream 获取一个流
func Stream(node string, md map[string]string) (stream Game_StreamClient, err error) {

	var c GameClient

	c, err = client.newClient(node)
	if err != nil {

		return
	}

	var ctx = context.Background()
	if md != nil {

		ctx = metadata.NewOutgoingContext(ctx, metadata.New(md))
	}

	stream, err = c.Stream(ctx)

	return
}

func streamCall(stream Game_StreamClient, service string, data interface{}, reply ...interface{}) error {

	b, err := codec.MsgPack(data)
	if err != nil {

		return err
	}

	msg := &GameMsg{ServiceName: service, Msg: b}

	err = stream.Send(msg)
	if err != nil {

		return err
	}

	ret, err := stream.Recv()
	if err != nil {

		return err
	}

	if len(ret.Error) != 0 {

		return errors.New(ret.Error)
	}

	if ret.Msg != nil && len(reply) > 0 {

		err = codec.UnMsgPack(ret.Msg, reply)
	}

	if err != nil {

		log.Printf("rpc stream call s=%s err=%s", service, err.Error())
	}

	return err
}

// Call 简单的grpc调用
func Call(node string, service string, data interface{}, reply ...interface{}) error {

	c, err := client.newClient(node)
	if err != nil {

		return err
	}

	b, err := codec.MsgPack(data)
	if err != nil {

		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()

	ret, err := c.Call(ctx, &GameMsg{ServiceName: service, Msg: b})
	if err != nil {

		return err
	}

	if len(ret.Error) > 0 {

		return errors.New(ret.Error)
	}

	if ret.Msg != nil && len(reply) > 0 {

		err = codec.UnMsgPack(ret.Msg, reply[0])
	}

	if err != nil {

		log.Printf("rpc call s=%s err=%s", service, err.Error())
	}

	return err
}
