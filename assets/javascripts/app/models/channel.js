App.Channel = DS.Model.extend({
  title:      DS.attr(),
  image_url:  DS.attr(),
  url:        DS.attr(),
  uri:        DS.attr(),
  description:DS.attr(),
  copyright:  DS.attr(),
  subscribed: DS.attr(),
  episodes:   function(){
    var _this = this;
    jQuery.getJSON("/api/channels/"+this.get('id')+"/episodes").then(function(data){
      var episodes = [];

      for (var i = data.episodes.length - 1; i >= 0; i--) {
        var episode = _this.store.push('episode', data.episodes[i]);
        episodes.push(episode)
      };
      $("#loading-page").parent().remove()
      _this.set('episodes', episodes);
    });
    return [];
  }.property("channel_id"),
  to_view:    DS.attr()
});


App.Subscription = DS.Model.extend({
  title:      DS.attr(),
  image_url:  DS.attr(),
  url:        DS.attr(),
  uri:        DS.attr(),
  to_view:    DS.attr()
});
