package mqtt

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/lomik/zapwriter"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"net/url"
	"os"
)

const schemaName = "mqtt"

var protocols = []string{"tcp", "ws", "wss"}

type MQTTOutput struct {
	Client   MQTT.Client
	topic    string
	sync     bool
	qos      byte
	retained bool

	errorLogger string
}

func (mq *MQTTOutput) writeSync(b []byte) (int, error) {
	token := mq.Client.Publish(mq.topic, mq.qos, mq.retained, b)
	if token.Error() != nil {
		zapwriter.Logger(mq.errorLogger).Error("writeSync", zap.Error(token.Error()))
		return 0, token.Error()
	}
	token.Wait()

	return len(b), nil
}

func (mq *MQTTOutput) writeAsync(b []byte) (int, error) {
	token := mq.Client.Publish(mq.topic, mq.qos, mq.retained, b)
	if token.Error() != nil {
		zapwriter.Logger(mq.errorLogger).Error("writeAsync", zap.Error(token.Error()))
		return 0, token.Error()
	}

	return len(b), nil
}

func (mq *MQTTOutput) Write(b []byte) (int, error) {
	// data race fix
	m := make([]byte, len(b))
	copy(m, b)
	if mq.sync {
		return mq.writeSync(m)
	}

	return mq.writeAsync(m)
}

func (mq *MQTTOutput) Sync() error {
	return nil
}

func (mq *MQTTOutput) Close() error {
	return nil
}

// topic, protocol, client_id are requited params.
// store is a path to dir where you want to save messages.
// if store is not set then memory is using

func validateParams(obj *zapwriter.DsnObj) error {
	protocol, _ := obj.String("protocol", "")
	if !inSlice(protocol, protocols) {
		return fmt.Errorf("protocol is required. Available values: %v", protocols)
	}

	topic, _ := obj.String("topic", "")
	if topic == "" {
		return fmt.Errorf("topic is required")
	}

	clientID, _ := obj.String("client_id", "")
	if clientID == "" {
		return fmt.Errorf("client_id is required")
	}

	return nil
}

func inSlice(key string, sl []string) bool {
	for _, x := range sl {
		if key == x {
			return true
		}
	}

	return false
}

func NewOutput(path string) (zapwriter.Output, error) {
	u, err := url.Parse(path)
	if err != nil {
		return nil, err
	}

	params := zapwriter.DSN(u.Query())
	sync, _ := params.Bool("sync", false)

	if validateErr := validateParams(params); validateErr != nil {
		return nil, validateErr
	}

	opts := MQTT.NewClientOptions()

	broker := u.Host
	protocol, _ := params.String("protocol", "")
	topic, _ := params.String("topic", "")
	clientID, _ := params.String("client_id", "")

	// required params
	opts.AddBroker(fmt.Sprintf("%s://%s", protocol, broker))
	opts.SetClientID(clientID)

	// optional params
	qos, err := params.Int("qos", 0)
	if err != nil {
		return nil, err
	}

	if qos < 0 || qos > 2 {
		return nil, fmt.Errorf("qos must be in (0,1,2)")
	}

	user, _ := params.String("user", "")
	password, _ := params.String("password", "")
	store, _ := params.String("store", "")
	if store != "" {
		// create dir if not exists
		if _, err := os.Stat(store); err != nil {
			if os.IsNotExist(err) {
				perms := os.FileMode(0770)
				makeErr := os.MkdirAll(store, perms)
				if makeErr != nil {
					return nil, makeErr
				}
			}
		} else {
			// check perms
			if accessErr := unix.Access(store, unix.R_OK|unix.W_OK); accessErr != nil {
				return nil, errors.Wrapf(accessErr, "check permission for path `%v`", store)
			}
		}
		opts.SetStore(MQTT.NewFileStore(store))
	}

	retained, _ := params.Bool("retained", false)

	opts.SetUsername(user)
	opts.SetPassword(password)

	client := MQTT.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	errorLogger, err := params.String("error_logger", "")
	if err != nil {
		return nil, err
	}

	output := &MQTTOutput{
		Client:   client,
		topic:    topic,
		sync:     sync,
		qos:      byte(qos),
		retained: retained,

		errorLogger: errorLogger,
	}

	return output, nil

}

func init() {
	zapwriter.RegisterScheme(schemaName, NewOutput)
}
