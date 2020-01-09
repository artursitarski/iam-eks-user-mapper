package main

import (
	"flag"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/kataras/golog"
	"gopkg.in/yaml.v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	iamGroups := flag.String("aws-iam-group", "", "--aws-iam-group=devs;jenkins")
	k8sCap := flag.String("k8s-cap", "", "--k8s-cap=system:masters,aggregate-to-admin;system:basic-user")
	flag.Parse()

	iamGroupsArr := strings.Split(*iamGroups, ";")
	k8sCapArr := strings.Split(*k8sCap, ";")
	if len(iamGroupsArr) != len(k8sCapArr) {
		panic("Amount of IAM groups and Kubernetes roles do not match up.")
	}

	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	for {
		cf, err := clientset.CoreV1().ConfigMaps("kube-system").Get("aws-auth", metav1.GetOptions{})
		if err != nil {
			panic(err.Error())
		}

		cf.Data["mapUsers"] = ""

		for index, iamGroup := range iamGroupsArr {
			users := getAwsIamGroup(iamGroup)
			var newConfig []MapUserConfig

			for _, user := range users.Users {
				newConfig = append(newConfig, MapUserConfig{
					UserArn:  *user.Arn,
					Username: *user.UserName,
					Groups:   strings.Split(k8sCapArr[index],","),
				})
			}

			roleStr, err := yaml.Marshal(newConfig)
			if err != nil {
				golog.Error(err)
			}
			cf.Data["mapUsers"] = strings.Join([]string{cf.Data["mapUsers"], string(roleStr)}, "")
		}

		newCF, err := clientset.CoreV1().ConfigMaps("kube-system").Update(cf)
		if err != nil {
			golog.Error(err)
		} else {
			golog.Info("successfully updated user roles")
			golog.Info(newCF)
		}

		time.Sleep(10 * time.Second)
	}
}

func getAwsIamGroup(groupName string) *iam.GetGroupOutput {
	sess := session.Must(session.NewSession(&aws.Config{}))
	iamClient := iam.New(sess)
	group, err := iamClient.GetGroup(&iam.GetGroupInput{
		GroupName: aws.String(groupName),
	})
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				golog.Error(iam.ErrCodeNoSuchEntityException, aerr.Error())
			case iam.ErrCodeServiceFailureException:
				golog.Error(iam.ErrCodeServiceFailureException, aerr.Error())
			default:
				golog.Error(aerr.Error())
			}
		}
	}
	return group
}
