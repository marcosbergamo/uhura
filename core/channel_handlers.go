package core

import (
	"net/http"
	"strconv"

	r "github.com/dukex/uhura/core/helper"
	"github.com/gorilla/mux"
)

func ReloadChannel(userId string, w http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	id := vars["id"]
	idI, _ := strconv.Atoi(id)

	TouchChannel(idI)
}

func SubscribeChannel(userId string, w http.ResponseWriter, request *http.Request) {
	var userChannel UserChannel

	vars := mux.Vars(request)
	id := vars["id"]

	channelId, _ := strconv.Atoi(id)
	userIdInt, _ := strconv.Atoi(userId)

	database.Table("user_channels").Where(UserChannel{ChannelId: int64(channelId), UserId: int64(userIdInt)}).FirstOrCreate(&userChannel)

	CacheUserSubscription(&userChannel)

	go func() {
		p := MIXPANEL.Identify(userId)
		p.Track("subscribed", map[string]interface{}{"Channel ID": id})
	}()

	go TouchChannel(channelId)

	GetChannel(userId, w, request)
}

func UnsubscribeChannel(userId string, w http.ResponseWriter, request *http.Request) {
	var userChannel UserChannel

	vars := mux.Vars(request)
	id := vars["id"]

	channelId, _ := strconv.Atoi(id)
	userIdInt, _ := strconv.Atoi(userId)

	go func() {
		p := MIXPANEL.Identify(userId)
		p.Track("unsubscribed", map[string]interface{}{"Channel ID": channelId})
	}()

	CACHE.Del(0, "s:"+id+":"+userId)
	CACHE.Del(0, "s:ids:"+userId)

	database.Table("user_channels").
		Where(UserChannel{ChannelId: int64(channelId), UserId: int64(userIdInt)}).
		Delete(&userChannel)
}

func GetChannel(userId string, w http.ResponseWriter, request *http.Request) {
	var (
		vars    = mux.Vars(request)
		id      = vars["id"]
		channel ChannelEntity
	)

	err := database.Table("channels").Where("channels.id = ?", id).First(&channel).Error

	if err != nil {
		w.WriteHeader(404)
		return
	}

	channel.SetEpisodesIds()
	channel.SetSubscription(userId)
	channel.SetToView(userId)

	r.ResponseJSON(w, 200, map[string]interface{}{"channel": channel})

	return
}

func GetChannelEpisodes(userId string, w http.ResponseWriter, request *http.Request) {
	var (
		userItems []int64
		vars      = mux.Vars(request)
		id        = vars["id"]
	)
	episodes := make([]EpisodeEntity, 0)

	database.Table("items").Where("items.channel_id = ?", id).Find(&episodes)

	database.Table("user_items").
		Where("channel_id = ?", id).
		Where("user_id = ?", userId).
		Where("viewed = TRUE").
		Pluck("item_id", &userItems)

	for i, episode := range episodes {
		episode.Listened = HasListened(userItems, episode.Id)
		episodes[i] = episode
	}
	r.ResponseJSON(w, 200, map[string]interface{}{"episodes": episodes})
}

func GetSubscriptions(userId string, w http.ResponseWriter, request *http.Request) {
	subscriptions := make([]ChannelEntity, 0)
	var ids []int

	subscriptionsCached, err := CacheGet("s:ids:"+userId, ids)

	if err == nil {
		var ok bool
		ids, ok = subscriptionsCached.([]int)
		if !ok {
			database.Table("user_channels").Where("user_channels.user_id = ?", userId).
				Pluck("user_channels.channel_id", &ids)
			go CacheSet("s:ids:"+userId, ids)
		}
	} else {
		database.Table("user_channels").Where("user_channels.user_id = ?", userId).
			Pluck("user_channels.channel_id", &ids)
		go CacheSet("s:ids:"+userId, ids)
	}

	if len(ids) > 0 {
		database.Table("channels").Where("channels.id in (?)", ids).Find(&subscriptions)
	}

	for i, channel := range subscriptions {
		subscriptions[i].Uri = channel.FixUri()
		go subscriptions[i].SetSubscribed(userId)
		subscriptions[i].SetEpisodesIds()
		subscriptions[i].SetToView(userId)
	}

	r.ResponseJSON(w, 200, map[string]interface{}{"subscriptions": subscriptions})
	return
}
