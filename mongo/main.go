package main

import (
	"context"
	"fmt"
	"gitee.com/sy_183/common/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

func main() {
	client, err := mongo.Connect(context.Background(),
		options.Client().
			ApplyURI("mongodb://192.168.80.79:27017").
			SetAuth(options.Credential{
				Username: "root",
				Password: "css66018",
			}).SetMaxPoolSize(1))
	if err != nil {
		panic(err)
	}
	logDB := client.Database("log")
	logCollection := logDB.Collection("log")

	i := int64(1)
	result := logCollection.FindOne(context.Background(), bson.D{}, options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}}))
	if err := result.Err(); err != nil {
		if err != mongo.ErrNoDocuments {
			panic(err)
		}
	} else {
		last := bson.D{}
		err = result.Decode(&last)
		//for _, e := range last {
		//	if e.Key == "_id" {
		//		i += e.Value.(int64)
		//	}
		//}
	}

	//ticker := time.NewTicker(time.Millisecond)
	for /*range ticker.C*/ {
		ds := make([]any, 0, 10)
		for j := 0; j < 40; j++ {
			ds = append(ds, bson.D{
				//{Key: "_id", Value: i},
				{Key: "T", Value: time.Now()},
				{Key: "L", Value: int64(log.InfoLevel)},
				{Key: "N", Value: "MONGODB日志模块"},
				{Key: "M", Value: "插入日志信息成功"},
				{Key: "日志条数", Value: int64(2)},
				{Key: "花费时间", Value: time.Millisecond * 10},
			})
			if i++; i%1000 == 0 {
				fmt.Printf("当前插入数据(%d)\n", i)
			}
		}
		_, err := logCollection.InsertMany(context.Background(), ds)
		if err != nil {
			panic(err)
		}
	}
}
