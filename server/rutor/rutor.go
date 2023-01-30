package rutor

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"github.com/agnivade/levenshtein"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"server/log"
	"server/rutor/models"
	"server/rutor/torrsearch"
	"server/rutor/utils"
	"server/settings"
	utils2 "server/torr/utils"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	torrs  []*models.TorrentDetails
	isStop bool
)

func Start() {
	go func() {
		if settings.BTsets.EnableRutorSearch {
			loadDB()
			updateDB()
			isStop = false
			for !isStop {
				for i := 0; i < 3*60*60; i++ {
					time.Sleep(time.Second)
					if isStop {
						return
					}
				}
				updateDB()
			}
		}
	}()
}

func Stop() {
	isStop = true
	torrs = nil
	torrsearch.NewIndex(nil)
	utils2.FreeOSMemGC()
	time.Sleep(time.Millisecond * 1500)
}

// https://github.com/yourok-0001/releases/raw/master/torr/rutor.ls
func updateDB() {
	log.TLogln("Update rutor db")
	fnTmp := filepath.Join(settings.Path, "rutor.tmp")
	out, err := os.Create(fnTmp)
	if err != nil {
		log.TLogln("Error create file rutor.tmp:", err)
		return
	}

	resp, err := http.Get("https://github.com/yourok-0001/releases/raw/master/torr/rutor.ls")
	if err != nil {
		log.TLogln("Error connect to rutor db:", err)
		out.Close()
		return
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		log.TLogln("Error download rutor db:", err)
		return
	}

	fnOrig := filepath.Join(settings.Path, "rutor.ls")

	md5Tmp := utils.MD5File(fnTmp)
	md5Orig := utils.MD5File(fnOrig)
	if md5Tmp != md5Orig {
		err = os.Remove(fnOrig)
		if err != nil && !os.IsNotExist(err) {
			log.TLogln("Error remove old rutor db:", err)
			return
		}
		err = os.Rename(fnTmp, fnOrig)
		if err != nil {
			log.TLogln("Error rename rutor db:", err)
			return
		}
		loadDB()
	} else {
		os.Remove(fnTmp)
	}
}

func loadDB() {
	log.TLogln("Load rutor db")
	buf, err := os.ReadFile(filepath.Join(settings.Path, "rutor.ls"))
	if err == nil {
		r := flate.NewReader(bytes.NewReader(buf))
		buf, err = io.ReadAll(r)
		r.Close()
		if err == nil {
			var ftors []*models.TorrentDetails
			err = json.Unmarshal(buf, &ftors)
			if err == nil {
				torrs = ftors
				log.TLogln("Index rutor db")
				torrsearch.NewIndex(torrs)
			} else {
				log.TLogln("Error read rutor db:", err)
			}
		} else {
			log.TLogln("Error read rutor db:", err)
		}
	} else {
		log.TLogln("Error load rutor db:", err)
	}
	utils2.FreeOSMemGC()
}

func Search(query string) []*models.TorrentDetails {
	if !settings.BTsets.EnableRutorSearch {
		return nil
	}
	matchedIDs := torrsearch.Search(query)
	if len(matchedIDs) == 0 {
		return nil
	}
	var list []*models.TorrentDetails
	for _, id := range matchedIDs {
		list = append(list, torrs[id])
	}

	hash := utils.ClearStr(query)

	sort.Slice(list, func(i, j int) bool {
		lhash := utils.ClearStr(strings.ToLower(list[i].Name+list[i].GetNames())) + strconv.Itoa(list[i].Year)
		lev1 := levenshtein.ComputeDistance(hash, lhash)
		lhash = utils.ClearStr(strings.ToLower(list[j].Name+list[j].GetNames())) + strconv.Itoa(list[j].Year)
		lev2 := levenshtein.ComputeDistance(hash, lhash)
		if lev1 == lev2 {
			return list[j].CreateDate.Before(list[i].CreateDate)
		}
		return lev1 < lev2
	})
	return list
}