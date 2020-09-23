package mqtt

import (
	"fmt"
	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/lomik/zapwriter"
	"net/url"
)

const schemaName = "mqtt"

var protocols = []string{"tcp", "ws", "wss"}

type MQTTOutput struct {
	Client MQTT.Client
	topic  string
	sync   bool

	errorLogger string
}

func (mq *MQTTOutput) writeSync(b []byte) (int, error) {
	token := mq.Client.Publish(mq.topic, byte(0), false, b)
	token.Wait()

	return len(b), nil
}

func (mq *MQTTOutput) writeAsync(b []byte) (int, error) {
	mq.Client.Publish(mq.topic, byte(0), false, b)

	return len(b), nil
}

func (mq *MQTTOutput) Write(b []byte) (int, error) {
	if mq.sync {
		return mq.writeSync(b)
	}

	return mq.writeAsync(b)
}

func (mq *MQTTOutput) Sync() error {
	return nil
}

func (mq *MQTTOutput) Close() error {
	panic("implement")
}

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

//broker = ip:port
//protocol = tcp|ws|wss
//user
//password
//client_id
//store?
//sync async?

//req: broker, protocol, client_id

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

	opts.SetUsername(user)
	opts.SetPassword(password)

	// todo: other params~?

	client := MQTT.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	errorLogger, err := params.String("error_logger", "")
	if err != nil {
		return nil, err
	}

	output := &MQTTOutput{
		Client: client,
		topic:  topic,
		sync:   sync,

		errorLogger: errorLogger,
	}

	return output, nil

}

func init() {
	zapwriter.RegisterScheme(schemaName, NewOutput)
}
