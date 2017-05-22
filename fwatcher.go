package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	fsevents "github.com/tywkeene/go-fsevents"
	"golang.org/x/sys/unix"
)

var dirptr, commandptr, eventsptr *string
var rootDir string
var inotifyFlags int
var options *fsevents.WatcherOptions
var eventsArr []string

var types = map[string]int{
	//基础flag
	"IN_ACCESS":        unix.IN_ACCESS,        //文件被访问
	"IN_ATTRIB":        unix.IN_ATTRIB,        //文件属性发生变化, e.g., permissions, timestamps, extended attributes, link count (since Linux 2.6.25), UID, GID, etc. (*).
	"IN_CLOSE_NOWRITE": unix.IN_CLOSE_NOWRITE, //以非write方式打开文件并关闭
	"IN_CLOSE_WRITE":   unix.IN_CLOSE_WRITE,   //以write方式打开文件并关闭
	"IN_CREATE":        unix.IN_CREATE,        //文件或目录被创建
	"IN_DELETE":        unix.IN_DELETE,        //文件或目录被删除
	"IN_DELETE_SELF":   unix.IN_DELETE_SELF,   //监控的根目录或文件本身被删除
	"IN_MODIFY":        unix.IN_MODIFY,        //文件内容被修改
	"IN_MOVED_FROM":    unix.IN_MOVED_FROM,    //文件移出被监测的目录
	"IN_MOVED_TO":      unix.IN_MOVED_TO,      //文件移入被监测的目录
	"IN_MOVE_SELF":     unix.IN_MOVE_SELF,     //监测的根目录或文件本身移动
	"IN_OPEN":          unix.IN_OPEN,          //文件被打开
	"IN_CLOEXEC":       unix.IN_CLOEXEC,
	//集合flag
	"IN_ALL_EVENTS": unix.IN_ALL_EVENTS, //	以上所有flag的集合"
	"IN_CLOSE":      unix.IN_CLOSE,      //IN_CLOSE_WRITE | IN_CLOSE_NOWRITE
	"IN_MOVE":       unix.IN_MOVE,       //IN_MOVED_FROM | IN_MOVED_TO
	//不常用的flag
	"IN_DONT_FOLLOW": unix.IN_DONT_FOLLOW, //不follow符号链接 (since 2.6.15)
	"IN_EXCL_UNLINK": unix.IN_EXCL_UNLINK, //当文件从监测目中unlink后，则不再报告该文件的相关event，比如监控/tmp使用 (since 2.6.36)
	"IN_MASK_ADD":    unix.IN_MASK_ADD,    //追加MASK到被监测的pathname
	"IN_ONESHOT":     unix.IN_ONESHOT,     //只监测一次
	"IN_ONLYDIR":     unix.IN_ONLYDIR,     //只监测目录
	//仅由read返回
	"IN_IGNORED":    unix.IN_IGNORED,    //inotify_rm_watch，文件被删除或者文件系统被umount
	"IN_ISDIR":      unix.IN_ISDIR,      //发生事件的是一个目录
	"IN_Q_OVERFLOW": unix.IN_Q_OVERFLOW, //Event队列溢出
	"IN_UNMOUNT":    unix.IN_UNMOUNT,    //文件系统unmount
}

var watcherMap = map[string]*fsevents.Watcher{}

func main() {
	dirptr = flag.String("dir", "/tmp", "directory to watch")
	eventsptr = flag.String("events", `IN_ALL_EVENTS,IN_ISDIR,IN_CLOSE,IN_MOVE,IN_EXCL_UNLINK`,
		`设置想要监听的事件flag;
	默认是:"IN_ALL_EVENTS,IN_ISDIR,IN_CLOSE,IN_MOVE,IN_EXCL_UNLINK"
	全部可用的事件flag如下:
	//基础flag
	"IN_ACCESS":        unix.IN_ACCESS,        //文件被访问
	"IN_ATTRIB":        unix.IN_ATTRIB,        //权限,时间戳,UID,GID,其他属性等等,link链接的数量 (since Linux 2.6.25) 
	"IN_CLOSE_NOWRITE": unix.IN_CLOSE_NOWRITE, //以非write方式打开文件并关闭
	"IN_CLOSE_WRITE":   unix.IN_CLOSE_WRITE,   //以write方式打开文件并关闭
	"IN_CREATE":        unix.IN_CREATE,        //文件或目录被创建
	"IN_DELETE":        unix.IN_DELETE,        //文件或目录被删除
	"IN_DELETE_SELF":   unix.IN_DELETE_SELF,   //监控的根目录或文件本身被删除
	"IN_MODIFY":        unix.IN_MODIFY,        //文件内容被修改
	"IN_MOVED_FROM":    unix.IN_MOVED_FROM,    //文件移出被监测的目录
	"IN_MOVED_TO":      unix.IN_MOVED_TO,      //文件移入被监测的目录
	"IN_MOVE_SELF":     unix.IN_MOVE_SELF,     //监测的根目录或文件本身移动
	"IN_OPEN":          unix.IN_OPEN,          //文件被打开
	"IN_CLOEXEC":       unix.IN_CLOEXEC,
	//集合flag
	"IN_ALL_EVENTS": unix.IN_ALL_EVENTS, //	以上所有flag的集合"
	"IN_CLOSE":      unix.IN_CLOSE,      //IN_CLOSE_WRITE | IN_CLOSE_NOWRITE
	"IN_MOVE":       unix.IN_MOVE,       //IN_MOVED_FROM | IN_MOVED_TO
	//不常用的flag
	"IN_DONT_FOLLOW": unix.IN_DONT_FOLLOW, //不follow符号链接 (since 2.6.15)
	"IN_EXCL_UNLINK": unix.IN_EXCL_UNLINK, //当文件从监测目中unlink后，则不再报告该文件的相关event，比如监控/tmp使用 (since 2.6.36)
	"IN_MASK_ADD":    unix.IN_MASK_ADD,    //追加MASK到被监测的pathname
	"IN_ONESHOT":     unix.IN_ONESHOT,     //只监测一次
	"IN_ONLYDIR":     unix.IN_ONLYDIR,     //只监测目录
	//仅由read返回
	"IN_IGNORED":    unix.IN_IGNORED,    //inotify_rm_watch，文件被删除或者文件系统被umount
	"IN_ISDIR":      unix.IN_ISDIR,      //发生事件的是一个目录
	"IN_Q_OVERFLOW": unix.IN_Q_OVERFLOW, //Event队列溢出
	"IN_UNMOUNT":    unix.IN_UNMOUNT,    //文件系统unmount`)
	commandptr = flag.String("cmd", "echo %f %t", "command to execute on change")
	flag.Parse()
	rootDir, _ = filepath.Abs(*dirptr)
	eventsArr = strings.FieldsFunc(*eventsptr, func(c rune) bool { return c == ',' })
	options = &fsevents.WatcherOptions{
		Recursive:       true,
		UseWatcherFlags: true,
	}
	inotifyFlags := (unix.IN_ALL_EVENTS | unix.IN_ISDIR |
		unix.IN_CLOSE | unix.IN_MOVE | unix.IN_EXCL_UNLINK)

	w, err := fsevents.NewWatcher(rootDir, inotifyFlags, options)
	if err != nil {
		panic(err)
	}

	log.Printf("Watched Dir : %v", *dirptr)
	log.Printf("Event Flags : %v", *eventsptr)
	log.Printf("Event Command : %v", *commandptr)
	log.Println("Waiting for events...")
	handleEvents(w)

}

func writeOutput(buf *bytes.Buffer) {
	if buf == nil {
		return
	}

	if len(buf.Bytes()) > 0 {
		fmt.Printf("%s\n", string(buf.Bytes()))
	}
}
func handleEvents(watcher *fsevents.Watcher) {
	watcher.StartAll()
	go watcher.Watch()
	for {
		select {
		case event := <-watcher.Events:
			if event.RawEvent.Len == 0 {
				continue
			}
			var eventType = getEventType(*event)
			inarray, _ := inArray(eventType, eventsArr)
			if event.IsDirCreated() {
				w, _ := fsevents.NewWatcher(event.Path, inotifyFlags, options)
				watcherMap[event.Path] = w
				log.Println("new dir watcher added : " + event.Path)
				go handleEvents(w)
			} else if event.IsDirRemoved() {
				_, ok := watcherMap[event.Path]
				if ok {
					log.Println("new dir watcher removed : " + event.Path)
					watcherMap[event.Path].StopAll()
					delete(watcherMap, event.Path)
				}
			}

			log.Printf("%s [%s] %d", event.Path, eventType, event.RawEvent.Len)

			if inarray {
				commandStr := strings.Replace(*commandptr, "%f", event.Path, 1)
				commandStr = strings.Replace(commandStr, "%t", eventType, 1)
				log.Println("Exec:" + commandStr)
				parts := strings.Fields(commandStr)
				cmd := exec.Command(parts[0], parts[1:]...)
				stdout := &bytes.Buffer{}
				stderr := &bytes.Buffer{}
				// write stdout to buffer
				cmd.Stdout = stdout
				cmd.Stderr = stderr
				err := cmd.Run()
				if err != nil {
					os.Stderr.WriteString(fmt.Sprintf("%s\n", err.Error()))
				}
				// write output
				writeOutput(stdout)
				writeOutput(stderr)
			}

			break
		case err := <-watcher.Errors:
			log.Println(err)
			break
		}
	}
}
func getEventType(e fsevents.FsEvent) string {
	var events []string
	var mask = e.RawEvent.Mask
	for key, value := range types {
		if (value & int(mask)) == value {
			events = append(events, key)
		}
	}
	if len(events) == 0 {
		events[0] = fmt.Sprintf("%d", e.RawEvent.Mask)
	}
	return strings.Join(events, ",")
}
func inArray(val string, array []string) (exists bool, index int) {
	exists = false
	index = -1

	for i, v := range array {
		if val == v {
			index = i
			exists = true
			return
		}
	}

	return
}
