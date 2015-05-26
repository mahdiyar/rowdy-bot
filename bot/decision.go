package bot

import(
    "bot/session"
    "bot/utils"
    "bot/http"
    "strings"
)

func BasicDecision(s *session.Session, follows int, likes int, intervals int){
    // Round robin the hashtags. Allows for manual weighting eg: [#dog,#dog,#cute] 
    posts := http.GetPosts(s,s.GetHashtag(intervals))

    // Go from end to reduce collision
    i := 19
    for (likes > 0 || follows > 0) && i >= 0 {

        // Process likes
        if likes > 0 {
            go http.LikePosts(s, posts.Data[i].Id)
            likes--

        // Doing this seperately reaches larger audience
        // Never exceeds 12/11 at a given time
        }else if follows > 0 {
            go http.FollowUser(s, posts.Data[i].Id)
            follows--
        }

        // Decrement
        i--
    }
}

func IntelligentDecision(s *session.Session, follows int, likes int, intervals int) {

    // Still do round robin, but this time the hashtags are smart
    posts := http.GetPosts(s,s.GetHashtag(intervals))
    next := make(chan *http.Posts)
    grp := make(chan *group)
    go sort(s, grp, follows, likes)
    go listen(s, grp, next, 0)
    next <- &posts
}

// Async heapsort, hope it works
func sort(s *session.Session, next chan *group, follows, likes int) {
    var instances []group
    count := 0
    x := 0
    min := 1.1
    for {
        select {
            case instance := <-next:

                x++
                // Catches up and thus done
                if x == utils.MAXPOSTGRAB * 20 || (min == 0 && count == follows + likes) {
                    i := 0
                    for (likes > 0 || follows > 0){

                        // Highest value for follows then do likes
                        if follows > 0 {
                            go http.FollowUser(s, instances[i].id)
                            follows--
                        }else if likes > 0 {
                            go http.LikePosts(s, instances[i].id)
                            likes--
                        }
                        i++
                    }
                    close(next)
                    return
                }

                if instance.id == "continue" || (instance.value <= min && count == follows + likes) {
                    continue
                }

                if min < instance.value {
                    if count == follows + likes {
                        min = instance.value
                    }
                } else {
                    if count < follows + likes {
                        min = instance.value
                    }                    
                }

                if count < follows + likes {
                    instances = append(instances, *instance)
                    count += 1
                } else {
                    instances[count - 1] = *instance
                }

                // Bubble sort
                for i := count - 2; i >= 0; i-- {
                    if instance.value > instances[i].value {
                        holder := instances[i]
                        instances[i] = *instance
                        instances[i + 1] = holder
                    } else {
                        break
                    }
                }
        }
    }
}

type group struct {
    value float64
    id string
}

// Async set up multi calls
func listen(s *session.Session,grp chan *group, next chan *http.Posts, calls int) {
    for {
        select {
            case posts := <-next:

                i := len(posts.Data) - 1
                go process(s, posts, i, grp)

                close(next)
                if calls == utils.MAXPOSTGRAB || posts.Pagination.Next_url == "" {
                    return
                }

                var batch http.Posts
                nxt := make(chan *http.Posts)
                batch = http.GetNextPost(s, posts.Pagination.Next_url)

                go listen(s, grp, nxt, calls + 1)
                nxt <- &batch
                return
        }
    }
}

func process(s *session.Session, posts *http.Posts, i int, grp chan *group){
    for i >= 0 {

        id := strings.Split(posts.Data[i].Id,"_")[1]
        if http.IsFollowing(s,id){
            grp <- &group{
                id:"continue",
            }            
            return
        }
        user  := http.GetUser(s, id)
        // Create perosn to get value
        person := session.Person{
            Followers: float64(user.Data.Counts.Follows),
            Following: float64(user.Data.Counts.Followed_by),
            Posts: float64(user.Data.Counts.Media),
        }

        grp <- &group{
            id:id,
            value: person.Sigmoid(s.GetTheta()),
        }

        i--
    }
}
