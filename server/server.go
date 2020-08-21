package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
}

type Message struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	Status            string            `json:"status"`
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []Alert           `json:"alerts"`
}

type At struct {
	AtMobiles []string `json:"atMobiles"`
	IsAtAll   bool     `json:"isAtAll"`
}

type DingTalkMarkdown struct {
	MsgType  string   `json:"msgtype"`
	At       At       `json:"at"`
	Markdown Markdown `json:"markdown"`
}

type Markdown struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

const layout = "Jan 2, 2006 at 3:04pm (MST)"

func SendToDingtalk(alertMessage Message, webhook string, atMobiles []string, isAtAll bool) error {
	groupKey := alertMessage.CommonLabels["group_id"]
	status := alertMessage.Status

	message := fmt.Sprintf("## HCaaS告警\n\n ### 告警组：%s（状态：%s)\n\n", groupKey, status)

	if _, ok := alertMessage.CommonLabels["alert_type"]; !ok {
		return errors.New("alert type is null")
	}

	var description string
	switch alertMessage.CommonLabels["alert_type"] {
	case "event":
		if _, ok := alertMessage.CommonLabels["event_type"]; !ok {
			return errors.New("event_type is null in commonLabels")
		}
		if _, ok := alertMessage.GroupLabels["resource_kind"]; !ok {
			return errors.New("resource kind is null in groupLabels")
		}
		description = fmt.Sprintf("\n > %s event of %s occuored\n\n", alertMessage.CommonLabels["event_type"], alertMessage.GroupLabels["resource_kind"])
	case "systemService":
		if _, ok := alertMessage.GroupLabels["component_name"]; !ok {
			return errors.New("component name is null in groupLabels")
		}
		description = fmt.Sprintf("\n > The system component %s is not running\n\n", alertMessage.GroupLabels["event_type"])
	case "nodeHealthy":
		if _, ok := alertMessage.GroupLabels["node_name"]; !ok {
			return errors.New("node name name is null in groupLabels")
		}
		description = fmt.Sprintf("\n > The kubelet on the node %s is not healthy\n\n", alertMessage.GroupLabels["node_name"])
	case "nodeCPU":
		if _, ok := alertMessage.GroupLabels["node_name"]; !ok {
			return errors.New("node name name is null in groupLabels")
		}
		if _, ok := alertMessage.CommonLabels["cpu_threshold"]; !ok {
			return errors.New("cpu threshold name is null in commonLabels")
		}
		description = fmt.Sprintf("\n > The CPU usage on the node %s is over %s%%\n\n", alertMessage.GroupLabels["node_name"], alertMessage.CommonLabels["cpu_threshold"])
	case "nodeMemory":
		if _, ok := alertMessage.GroupLabels["node_name"]; !ok {
			return errors.New("node name name is null in groupLabels")
		}
		if _, ok := alertMessage.CommonLabels["mem_threshold"]; !ok {
			return errors.New("mem threshold name is null in commonLabels")
		}
		description = fmt.Sprintf("\n > The memory usage on the node %s is over %s%%\n\n", alertMessage.GroupLabels["node_name"], alertMessage.CommonLabels["mem_threshold"])
	case "podNotScheduled":
		if _, ok := alertMessage.GroupLabels["pod_name"]; !ok {
			return errors.New("pod name name is null in groupLabels")
		}
		var pod string
		if namespace, ok := alertMessage.GroupLabels["namespace"]; ok {
			pod = namespace + alertMessage.GroupLabels["pod_name"]
		} else {
			pod = alertMessage.GroupLabels["pod_name"]
		}
		description = fmt.Sprintf("\n > The Pod %s is not scheduled\n\n", pod)
	case "podNotRunning":
		if _, ok := alertMessage.GroupLabels["pod_name"]; !ok {
			return errors.New("pod name name is null in groupLabels")
		}
		var pod string
		if namespace, ok := alertMessage.GroupLabels["namespace"]; ok {
			pod = namespace + alertMessage.GroupLabels["pod_name"]
		} else {
			pod = alertMessage.GroupLabels["pod_name"]
		}
		description = fmt.Sprintf("\n > The Pod %s is not running\n\n", pod)
	case "podRestarts":
		if _, ok := alertMessage.GroupLabels["pod_name"]; !ok {
			return errors.New("pod name name is null in groupLabels")
		}
		if _, ok := alertMessage.CommonLabels["restart_times"]; !ok {
			return errors.New("restart times is null in commonLabels")
		}
		if _, ok := alertMessage.CommonLabels["restart_interval"]; !ok {
			return errors.New("restart interval is null in commonLabels")
		}
		var pod string
		if namespace, ok := alertMessage.GroupLabels["namespace"]; ok {
			pod = namespace + alertMessage.GroupLabels["pod_name"]
		} else {
			pod = alertMessage.GroupLabels["pod_name"]
		}
		description = fmt.Sprintf("\n > The Pod %s restarts %s times in %s sec\n\n", pod, alertMessage.CommonLabels["restart_times"], alertMessage.CommonLabels["restart_interval"])
	case "workload":
		if _, ok := alertMessage.GroupLabels["workload_name"]; !ok {
			return errors.New("workload name is null in groupLabels")
		}
		if _, ok := alertMessage.CommonLabels["available_percentage"]; !ok {
			return errors.New("available percentage is null in commonLabels")
		}
		var workload string
		if namespace, ok := alertMessage.GroupLabels["workload_namespace"]; ok {
			workload = namespace + alertMessage.GroupLabels["workload_name"]
		} else {
			workload = alertMessage.GroupLabels["workload_name"]
		}
		description = fmt.Sprintf("\n > The workload %s has available replicas less than %s%%\n\n", workload, alertMessage.CommonLabels["available_percentage"])
	case "metric":
		if _, ok := alertMessage.CommonLabels["alert_name"]; !ok {
			return errors.New("alert name is null in commonLabels")
		}
		description = fmt.Sprintf("\n > The metric %s crossed the threshold\n\n", alertMessage.CommonLabels["alert_name"])
	default:
		return errors.New("invalid alert type")
	}

	message += description

	for _, alert := range alertMessage.Alerts {
		if alert.Status != "firing" {
			continue
		}
		message += "-----\n"

		for k, v := range alert.Labels {
			message += fmt.Sprintf("- %s : %s\n", k, v)
		}
		message += fmt.Sprintf("- 起始时间：%s\n", alert.StartsAt.Format(layout))
	}

	dingtalkText := DingTalkMarkdown{
		MsgType: "markdown",
		At: At{
			AtMobiles: atMobiles,
			IsAtAll:   isAtAll,
		},
		Markdown: Markdown{
			Title: fmt.Sprintf("HCaaS 告警组：%s（状态：%s）", groupKey, status),
			Text:  message,
		},
	}

	data, err := json.Marshal(dingtalkText)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, webhook, bytes.NewBuffer(data))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	client := http.Client{Transport: tr}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		log.Printf("[ERROR] %s", resp.Header)
	}

	log.Printf("[INFO] Alert message sent to %s successfully", webhook)
	_ = resp.Body.Close()
	return nil
}

func ReceiveAndSend(w http.ResponseWriter, req *http.Request) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, err)
		log.Printf("[ERROR] %s", err)
		return
	}

	alertMessage := Message{}
	_ = json.Unmarshal(body, &alertMessage)

	err = req.ParseForm()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, err)
		return
	}

	if _, ok := req.Form["webhook"]; !ok {
		log.Print("[ERROR] url argument \"webhook\" is null")
		return
	}
	if _, ok := req.Form["isatall"]; !ok {
		log.Print("[ERROR] url argument \"isatall\" is null")
		return
	}
	webhook := req.Form["webhook"][0]
	atmobiles := req.Form["atmobiles"]
	isatall, _ := strconv.ParseBool(req.Form["isatall"][0])

	err = SendToDingtalk(alertMessage, webhook, atmobiles, isatall)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, err)
		log.Printf("[ERROR] %s", err)
		return
	}

	_, _ = fmt.Fprint(w, "Alert sent successfully")
}
