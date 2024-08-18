package logimport

import (
	"bufio"
	"errors"
	"io"
	"os"

	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	providerv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/provider/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	owngrpc "github.com/mpapenbr/go-racelogger/pkg/grpc"
	"github.com/mpapenbr/go-racelogger/pkg/grpc/logger"
	"github.com/mpapenbr/go-racelogger/pkg/util"
)

var (
	eventKey    = ""
	replaceData = false
)

func NewImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <file>",
		Short: "import race from previous logged grpc messages file",
		Args:  cobra.ExactArgs(1),

		Run: func(cmd *cobra.Command, args []string) {
			doImport(args[0])
		},
	}
	cmd.Flags().StringVarP(&config.DefaultCliArgs().Token,
		"token",
		"t",
		"",
		"Dataprovider token")
	cmd.Flags().BoolVar(&config.DefaultCliArgs().DoNotPersist,
		"do-not-persist",
		false,
		"do not persist the recorded data (used for debugging)")
	cmd.Flags().StringVar(&eventKey,
		"event-key",
		"",
		"Import data with this event key (default is to use the event key in the file)")
	cmd.Flags().BoolVar(&replaceData,
		"replace-data",
		false,
		"replace existing data on server with imported data")

	return cmd
}

type importProc struct {
	conn          *grpc.ClientConn
	f             *os.File
	m             *logger.MsgLogger
	dpc           *owngrpc.DataProviderClient
	recordingMode providerv1.RecordingMode
	replaceData   bool
	eventKey      string
}

func newImportProc(conn *grpc.ClientConn, f *os.File) *importProc {
	ret := &importProc{
		conn:          conn,
		f:             f,
		recordingMode: providerv1.RecordingMode_RECORDING_MODE_PERSIST,
	}
	ret.m = logger.NewMsgLogger(logger.WithReader(bufio.NewReader(f)))
	ret.dpc = owngrpc.NewDataProviderClient(
		owngrpc.WithConnection(conn),
		owngrpc.WithToken(config.DefaultCliArgs().Token),
	)
	return ret
}

func doImport(fn string) {
	conn, err := util.ConnectGrpc(config.DefaultCliArgs())
	if err != nil {
		log.Error("error connecting to grpc server", log.ErrorField(err))
		return
	}
	f, err := os.Open(fn)
	if err != nil {
		log.Error("error opening file: %v", log.ErrorField(err))
		return
	}
	defer f.Close()
	proc := newImportProc(conn, f)
	if config.DefaultCliArgs().DoNotPersist {
		proc.recordingMode = providerv1.RecordingMode_RECORDING_MODE_DO_NOT_PERSIST
	}
	proc.replaceData = replaceData
	proc.eventKey = eventKey
	defer proc.close()
	proc.process()
}

func (p *importProc) process() {
	i := 0
	for {
		msg, err := p.m.ReadNext()
		if errors.Is(err, io.EOF) {
			return // done
		}
		if err != nil {
			log.Error("error reading message", log.ErrorField(err))
			break
		}
		log.Debug("message",
			log.Int("i", i),
			log.String("name", string(msg.Descriptor().Name())))
		if err := p.sendMesage(msg); err != nil {
			log.Error("error sending message", log.ErrorField(err))
			return
		}
		i++
	}
}

//nolint:cyclop // by design
func (p *importProc) sendMesage(msg protoreflect.Message) error {
	switch msg.Interface().(type) {
	case *providerv1.RegisterEventRequest:
		req, _ := msg.Interface().(*providerv1.RegisterEventRequest)
		if p.eventKey != "" {
			req.Event.Key = p.eventKey
		}
		if p.replaceData {
			if err := p.dpc.DeleteEvent(req.Event.Key); err != nil {
				if status.Code(err) != codes.NotFound {
					return err
				}
			}
		}
		_, err := p.dpc.RegisterProvider(req.Event, req.Track, p.recordingMode)
		return err
	case *providerv1.UnregisterEventRequest:
		req, _ := msg.Interface().(*providerv1.UnregisterEventRequest)
		eventKey := req.EventSelector.GetKey()
		if p.eventKey != "" {
			eventKey = p.eventKey
		}
		return p.dpc.UnregisterProvider(eventKey)
	case *racestatev1.PublishStateRequest:
		req, _ := msg.Interface().(*racestatev1.PublishStateRequest)
		p.updateEventSelector(req.Event)
		return p.dpc.PublishState(req)
	case *racestatev1.PublishDriverDataRequest:
		req, _ := msg.Interface().(*racestatev1.PublishDriverDataRequest)
		p.updateEventSelector(req.Event)
		return p.dpc.PublishDriverData(req)
	case *racestatev1.PublishSpeedmapRequest:
		req, _ := msg.Interface().(*racestatev1.PublishSpeedmapRequest)
		p.updateEventSelector(req.Event)
		return p.dpc.PublishSpeedmap(req)
	}
	return nil
}

func (p *importProc) updateEventSelector(sel *commonv1.EventSelector) {
	if p.eventKey != "" {
		sel.Arg = &commonv1.EventSelector_Key{Key: p.eventKey}
	}
}

func (p *importProc) close() {
	p.dpc.Close()
}
