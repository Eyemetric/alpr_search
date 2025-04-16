package main

import (
	"context"
	_ "context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "log"
	"time"
	_ "time"
)

const (
	PRESIGN_EXPIRES = 360 * time.Minute //6hr cache.
)

type Wasabi struct {
	s3Client      *s3.Client
	presignClient *s3.PresignClient
}

// creates a secure but publicly accessible image link with a 6hr expiration
func (w *Wasabi) PresignUrl(bucket, objectKey string) (string, error) {
	//we need the bucket and the object.
	//prepare presign request input

	getObjInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(objectKey),
	}

	presignResult, err := w.presignClient.PresignGetObject(context.TODO(), getObjInput, func(po *s3.PresignOptions) {
		po.Expires = PRESIGN_EXPIRES
	})

	if err != nil {
		return "", err
	}
	return presignResult.URL, nil

}

// return a struct that wraps the aws S3 client for Wasabi
func NewWasabi(s3Host, s3Region string) (*Wasabi, error) {
	s3Endpoint := fmt.Sprintf("https://%s", s3Host)
	usePathStyle := true
	//lets the sdk know we aren't calling official aws servers. GOOD LORD! This aws api is HOT TRASH!
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           s3Endpoint,
				SigningRegion: s3Region,
				//where the endpoint came from
				Source: aws.EndpointSourceCustom,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}

	})

	//TODO: update context
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(s3Region),
		config.WithEndpointResolverWithOptions(customResolver))
	if err != nil {
		return nil, err
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = usePathStyle
	})

	//all of the above horseshit just to get this! lol
	presignClient := s3.NewPresignClient(s3Client)

	wasabi := &Wasabi{
		s3Client:      s3Client,
		presignClient: presignClient,
	}

	return wasabi, nil
}
