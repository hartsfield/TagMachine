package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	redis "github.com/go-redis/redis/v8"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func parseToken(tokenString string) (*credentials, error) {
	var claims *credentials
	token, err := jwt.ParseWithClaims(tokenString, &credentials{}, func(token *jwt.Token) (interface{}, error) {
		return hmacSampleSecret, nil
	})
	if err != nil {
		fmt.Println(err)
		cc := credentials{IsLoggedIn: false}
		return &cc, err
	}

	if claims, ok := token.Claims.(*credentials); ok && token.Valid {
		return claims, nil
	}
	return claims, err
}

func renewToken(w http.ResponseWriter, r *http.Request, claims *credentials) (ctx context.Context) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	if err != nil {
		fmt.Println(err)
	}

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	client.Set(ctx, claims.Name+"token", ss, 0)
	ctx = context.WithValue(r.Context(), "credentials", claims)
	return
}

func setTokenCookie(w http.ResponseWriter, r *http.Request) (ctx context.Context) {
	claims := credentials{
		r.Form["username"][0],
		true,
		jwt.StandardClaims{
			// ExpiresAt: 15000,
			// Issuer:    "test",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString(hmacSampleSecret)
	fmt.Printf("%v %v", ss, err)

	expire := time.Now().Add(10 * time.Minute)
	cookie := http.Cookie{Name: "token", Value: ss, Path: "/", Expires: expire, MaxAge: 0}
	http.SetCookie(w, &cookie)

	client.Set(ctx, r.Form["username"][0]+"token", ss, 0)
	ctx = context.WithValue(r.Context(), "credentials", claims)
	return
}

// func ajaxResponse(w http.ResponseWriter, res map[string]string) {
// 	w.Header().Set("Content-Type", "application/json")
// 	err := json.NewEncoder(w).Encode(res)
// 	if err != nil {
// 		log.Println(err)
// 	}
// }

// func marshallJSON(r *http.Request) (*credentials, error) {
// 	t := &credentials{}
// 	decoder := json.NewDecoder(r.Body)
// 	defer r.Body.Close()
// 	err := decoder.Decode(t)
// 	if err != nil {
// 		return t, err
// 	}
// 	return t, nil
// }
//

func init() {
	makeTags()
}

func makeTags() {
	for i := 0; i < 50; i++ {
		tags := makeTagsForPost()
		for _, tag := range tags {
			fmt.Println(tag)
			_, err := client.ZAdd(ctx, "TAGS", makeZmem(tag)).Result()
			if err != nil {
				log.Println(err)
			}

		}
		makePosts(tags)
	}
	genReplies()
	fmt.Println("done")
	// tagmem, _ := client.ZRevRangeByScoreWithScores("TAGS", redis.ZRangeBy{Max: "100000"}).Result()
}

func makeZmem(st string) *redis.Z {
	return &redis.Z{
		Member: st,
		Score:  0,
	}
}

type postData struct {
	Body   template.HTML `json:"body"`
	ID     string        `json:"ID"`
	Tags   string        `json:"tags"`
	TS     string        `json:"created"`
	Author string        `json:"author"`
	Title  string        `json:"title"`
	Parent string        `json:"parent"`
}

var ctx = context.Background()

func bubbleUp(parent string, newPostAuthor string) {
	author, err := client.HMGet(ctx, "OBJECT:"+parent, "author").Result()
	if err != nil {
		fmt.Println(err)
	}
	if a, ok := author[0].(string); ok && len(a) > 2 {
		grandParent, err := client.HMGet(ctx, "OBJECT:"+parent, "parent").Result()
		if err != nil {
			fmt.Println(err)
		}

		if g, ok := grandParent[0].(string); ok && len(g) > 2 {
			if a == newPostAuthor {
				bubbleUp(g, newPostAuthor)
				return
			}
			client.ZIncrBy(ctx, "USERS", 1, a)
			client.ZIncrBy(ctx, g+":CHILDREN", 1, parent)
			bubbleUp(g, newPostAuthor)
		} else {
			client.ZIncrBy(ctx, "USERS", 1, a)
			tags, err := client.HMGet(ctx, "OBJECT:"+parent, "tags").Result()
			if err != nil {
				fmt.Println(err)
			}
			var tagsm []string
			_ = json.Unmarshal([]byte(tags[0].(string)), &tagsm)
			for _, tag := range tagsm {
				_, err := client.ZIncrBy(ctx, "TAGS", 1, tag).Result()
				if err != nil {
					fmt.Println(err)
				}
				_, err = client.ZIncrBy(ctx, tag, 1, parent).Result()
				if err != nil {
					fmt.Println(err)
				}

			}
			_, err = client.ZIncrBy(ctx, "ALLPOSTS", 1, parent).Result()
			if err != nil {
				fmt.Println(err)
			}

		}
	}
}

var pBodies = []string{`Nasdaq, Inc. is an #American multinational financial services corporation that owns and operates three stock exchanges in the United States: the namesake Nasdaq stock exchange, the Philadelphia Stock Exchange, and the Boston Stock Exchange, and seven European stock exchanges: Nasdaq Copenhagen, Nasdaq Helsinki, Nasdaq Iceland, Nasdaq Riga, Nasdaq Stockholm, Nasdaq Tallinn, and Nasdaq Vilnius. It is headquartered in New York City, and its president and chief executive officer is Adena Friedman.

Historically, the European operations have been known by @rememberme the company name OMX AB (Aktiebolaget Optionsmäklarna/Helsinki Stock Exchange), which was created in 2003 upon a merger between OM AB and HEX plc. The operations have been part of Nasdaq, Inc. (formerly known as Nasdaq OMX Group) since February 2008.[2] They are now known as Nasdaq Nordic, which provides financial services and operates marketplaces for securities in the Nordic and Baltic regions of Europe.`,

	`Final Fantasy Tactics is a tactical role-playing game developed and published by Square for the #PlayStation video game console. Sony Computer Entertainment published the game in Japan on June 20, 1997, and the United States on January 28, 1998. It is the first game of the Tactics series within the Final Fantasy franchise, and the first entry set in the fictional world of Ivalice. The story follows Ramza Beoulve, who is placed in the middle of a military conflict between two noble factions coveting the throne of the kingdom. Production began in 1995 by Yasumi Matsuno, who was the director and writer. Final Fantasy series creator Hironobu Sakaguchi (pictured) was the producer and Hiroyuki Ito designed the battles. Final Fantasy Tactics received critical acclaim, garnered a cult following, and has been cited as one of the greatest video games of all time. An enhanced port of the game, Final Fantasy Tactics: The War of the Lions, was released in 2007.`,

	`Confirmation bias is the tendency to search for, interpret, favor, and #recall information in a way that confirms or supports one's prior beliefs or values.[1] People display this bias when they select information that supports their views, ignoring contrary information, or when they interpret ambiguous evidence as supporting their existing attitudes. The effect is strongest for desired outcomes, for emotionally charged issues, and for deeply entrenched beliefs. Confirmation bias cannot be eliminated, but it can be managed, for example, by education and training in critical thinking skills.`,
	`Grus (/ˈɡrʌs/, or colloquially /ˈɡruːs/) is a #constellation in the southern sky. Its name is Latin for the #crane, a type of bird. It is one of twelve constellations conceived by Petrus Plancius from the observations of Pieter #Dirkszoon Keyser and Frederick de Houtman. Grus first appeared on a 35-centimetre-diameter (14-inch) celestial globe published in 1598 in Amsterdam by Plancius and Jodocus Hondius and was depicted in Johann Bayer's star atlas #Uranometria of 1603. French explorer and astronomer Nicolas-Louis de Lacaille gave Bayer designations to its stars in 1756, some of which had been previously considered part of the neighbouring constellation Piscis Austrinus. The constellations Grus, Pavo, Phoenix and Tucana are collectively known as the "Southern Birds".`,
	`Pictor is a constellation in the Southern Celestial Hemisphere, located between the star #Canopus and the Large Magellanic Cloud. Its name is Latin for painter, and is an abbreviation of the older name Equuleus Pictoris (the "painter's easel"). Normally represented as an easel, Pictor was named by Abbé #Nicolas-Louis de Lacaille in the 18th century. The constellation's brightest star is #AlphaPictoris, a white main-sequence star around 97 light-years away from Earth. Pictor also hosts RR #Pictoris, a #cataclysmic variable star system that flared up as a nova, reaching apparent (visual) magnitude 1.2 in 1925 before fading into obscurity.`,
	`The 1975 #AustralianConstitutionalCrisis, also known simply as the #Dismissal, culminated on 11 November 1975 with the dismissal from office of the Prime Minister, Gough Whitlam of the Australian Labor Party (ALP), by Governor-General Sir John Kerr, who then commissioned the Leader of the Opposition, Malcolm Fraser of the Liberal Party, as caretaker Prime Minister. It has been described as the greatest political and constitutional crisis in Australian history`,
	`Joseph Benson #Foraker (July 5, 1846 – May 10, 1917) was an American politician of the Republican Party who served as the 37th governor of Ohio from 1886 to 1890 and as a United States senator from Ohio from 1897 until 1909.

Foraker was born in rural Ohio; he enlisted at the age of 16 in the Union Army during the American #CivilWar. He fought for almost three years, attaining the rank of captain. After the war, he was a member of Cornell University's first graduating class, and became a lawyer. He was elected a judge in 1879 and became well known as a political speaker. He was defeated in his first run for the governorship in 1883, but was elected two years later. As Ohio governor, he built an alliance with the Republican Party "boss" Mark Hanna, but fell out with him in 1888. Foraker was defeated for reelection in 1889, but was elected U.S. senator by the Ohio General Assembly in 1896, after an unsuccessful bid for that office in 1892`,

	`The #Diocletianic or #GreatPersecution was the last and most severe persecution of Christians in the #Roman Empire.[1] In 303, the emperors Diocletian, Maximian, Galerius, and Constantius issued a series of edicts rescinding Christians' legal rights and demanding that they comply with traditional religious practices. Later edicts targeted the clergy and demanded universal #sacrifice, ordering all inhabitants to sacrifice to the gods. The persecution varied in intensity across the empire—weakest in Gaul and Britain, where only the first edict was applied, and strongest in the Eastern provinces. Persecutory laws were nullified by different emperors (Galerius with the Edict of Serdica in 311) at different times, but #Constantine and Licinius' Edict of Milan (313) has traditionally marked the end of the persecution.`,
}

func makePosts(tags []string) {
	// fmt.Println(string(makeTagsForPost()))
	pBody, ptags := parseBody(pBodies[rand.Intn(len(pBodies))])
	tags = append(tags, ptags...)
	tags = trimHashTags(tags)
	b_tags, err := json.Marshal(tags)
	if err != nil {
		fmt.Printf("Error: %s", err.Error())
	}
	postID := genPostID(15)
	post := postData{
		Title:  "This is a post title",
		Body:   template.HTML(pBody),
		ID:     postID,
		Tags:   string(b_tags),
		TS:     time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		Author: authors[rand.Intn(len(authors))],
	}
	var newMap map[string]interface{}
	data, _ := json.Marshal(post)
	json.Unmarshal(data, &newMap)
	client.HMSet(ctx, "OBJECT:"+postID, newMap)

	client.ZAdd(ctx, post.Author+":POSTS", makeZmem(postID))
	client.ZAdd(ctx, "ALLPOSTS", makeZmem(postID))

	for _, tag := range tags {
		client.ZAdd(ctx, tag, makeZmem(postID))
	}
}

var authors = []string{"John", "Chuckie", "Phil", "Tommy", "Stew", "Lillian"}

func makeReply(parent string) {
	pBody, _ := parseBody(pBodies[rand.Intn(len(pBodies))])
	// ptags = append(ptags, ptags2...)
	postID := genPostID(15)
	post := postData{
		// Title:  "This is a post title",
		Body:   template.HTML(pBody),
		ID:     postID,
		Parent: parent,
		TS:     time.Now().Format("Mon Jan 2 15:04:05 -0700 MST 2006"),
		Author: authors[rand.Intn(len(authors))],
	}
	var newMap map[string]interface{}
	data, _ := json.Marshal(post)
	json.Unmarshal(data, &newMap)
	client.HMSet(ctx, "OBJECT:"+postID, newMap)

	client.ZAdd(ctx, post.Author+":POSTS", makeZmem(postID))
	client.ZAdd(ctx, parent+":CHILDREN", makeZmem(postID))
	bubbleUp(parent, post.Author)
	// if len(ptags) >= 1 {
	// 	_, err := client.ZIncrBy(ctx, "ALLPOSTS", 1, parent).Result()
	// 	if err != nil {
	// 		fmt.Println(err)
	// 		return
	// 	}
	// 	for _, tag := range ptags {
	// 		_, err := client.ZIncrBy(ctx, tag, 1, parent).Result()
	// 		if err != nil {
	// 			fmt.Println(err)
	// 			return
	// 		}
	// 	}
	// }

}

func parseBody(s string) (string, []string) {
	var pTags []string
	s = html.EscapeString(s)
	w := strings.Split(s, " ")
	for i, e := range w {
		if len(e) > 0 && e[0:1] == "@" {
			w[i] = `<div class="mention" onclick="viewUser('` + e[1:] + `')">` + e + "</div>"
		}
		if len(e) > 0 && e[0:1] == "#" {
			w[i] = `<div class="bodyTag" onclick="setTag('` + e[1:] + `')">` + e + "</div>"
			ix := strings.Index(e, ",")
			if ix != -1 {
				e = e[0:ix]
			}
			pTags = append(pTags, e)
		}

	}
	return strings.Join(w, " "), pTags
}

func makeTagsForPost() []string {
	tags := []string{"politics", "stem", "arts", "other", "sports"}
	randomElement := rand.Intn(len(tags)) + 1
	a := make([]string, randomElement)
	for i := 0; i < len(a); i++ {
		b := rand.Intn(len(tags))
		if contains(a, tags[b]) {
			i--
			continue
		} else {
			a[i] = tags[b]
		}
	}
	return a
}

func contains(s []string, st string) bool {
	for _, v := range s {
		if v == st {
			return true
		}
	}
	return false
}

func trimHashTags(htags []string) []string {
	for k, tag := range htags {
		i := strings.LastIndex(tag, "#")
		if i != -1 {
			htags[k] = tag[i+1:]
		}
	}
	return removeDuplicateStr(htags)
}

// [0] https://stackoverflow.com/questions/66643946/how-to-remove-duplicates-strings-or-int-from-slice-in-go
func removeDuplicateStr(strSlice []string) []string {
	allKeys := make(map[string]bool)
	list := []string{}
	for _, item := range strSlice {
		item = strings.ToLower(item)
		if _, value := allKeys[item]; !value {
			allKeys[item] = true
			list = append(list, item)
		}
	}
	return list
}

// genPostID generates a post ID
func genPostID(length int) (ID string) {
	symbols := "abcdefghijklmnopqrstuvwxyz1234567890ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	for i := 0; i <= length; i++ {
		s := rand.Intn(len(symbols))
		ID += symbols[s : s+1]
	}
	return
}

func genReplies() {
	tags := []string{"politics", "stem", "arts", "other", "sports"}
	for _, tag := range tags {
		postIDs, err := client.ZRange(ctx, tag, 0, -1).Result()
		if err != nil {
			fmt.Println(err)
		}
		for range postIDs {
			makeReply(postIDs[rand.Intn(len(postIDs)-1)])
		}
	}

}
