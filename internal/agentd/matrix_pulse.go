package agentd

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"manifold/internal/persistence"
	pulsecore "manifold/internal/pulse"
)

const (
	defaultMatrixPulsePollInterval = 5 * time.Minute
	defaultMatrixPulseLease        = 4 * time.Minute
)

type pulseRuntime struct {
	app      *app
	store    persistence.PulseStore
	service  *pulsecore.Service
	interval time.Duration
	lease    time.Duration
	cancel   context.CancelFunc
	done     chan struct{}
}

type pulseTaskRun struct {
	room persistence.PulseRoom
	task persistence.PulseTask
}

func newPulseRuntime(app *app, store persistence.PulseStore) *pulseRuntime {
	return &pulseRuntime{
		app:      app,
		store:    store,
		service:  pulsecore.NewService(),
		interval: defaultMatrixPulsePollInterval,
		lease:    defaultMatrixPulseLease,
	}
}

func (r *pulseRuntime) Start(ctx context.Context) error {
	if r == nil || r.store == nil || r.app == nil || r.app.matrixGateway == nil || r.app.cfg == nil || !r.app.cfg.Matrix.Enabled {
		return nil
	}
	if r.cancel != nil {
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		defer close(r.done)
		r.run(runCtx)
	}()
	return nil
}

func (r *pulseRuntime) Close() error {
	if r == nil {
		return nil
	}
	cancel := r.cancel
	done := r.done
	r.cancel = nil
	r.done = nil
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	return nil
}

func (r *pulseRuntime) run(ctx context.Context) {
	if err := r.pollOnce(ctx); err != nil && ctx.Err() == nil {
		log.Warn().Err(err).Msg("matrix_pulse_initial_poll_failed")
	}
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.pollOnce(ctx); err != nil && ctx.Err() == nil {
				log.Warn().Err(err).Msg("matrix_pulse_poll_failed")
			}
		}
	}
}

func (r *pulseRuntime) pollOnce(ctx context.Context) error {
	rooms, err := r.store.ListRooms(ctx, "")
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	jobsByTarget := make(map[string][]pulseTaskRun)
	for _, room := range rooms {
		if !room.Enabled {
			continue
		}
		tasks, err := r.store.ListTasks(ctx, room.RoomID, room.RouteTarget)
		if err != nil {
			log.Warn().Str("room_id", room.RoomID).Str("target", room.RouteTarget).Err(err).Msg("matrix_pulse_list_tasks_failed")
			continue
		}
		plan := r.service.EvaluateRoom(now, room, tasks, room.RouteTarget)
		if len(plan.DueTasks) == 0 {
			continue
		}
		for _, task := range plan.DueTasks {
			target := task.RouteTarget
			if target == "" {
				target = room.RouteTarget
			}
			jobsByTarget[target] = append(jobsByTarget[target], pulseTaskRun{room: room, task: task})
		}
	}
	if len(jobsByTarget) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	for target, jobs := range jobsByTarget {
		wg.Go(func() {
			for _, job := range jobs {
				if ctx.Err() != nil {
					return
				}
				r.runPulseTask(ctx, now, target, job)
			}
		})
	}
	wg.Wait()
	return nil
}

func (r *pulseRuntime) runPulseTask(ctx context.Context, now time.Time, target string, job pulseTaskRun) {
	room := job.room
	task := job.task
	claimToken := uuid.NewString()
	claimed, err := r.store.ClaimRoom(ctx, room.RoomID, room.RouteTarget, claimToken, now.Add(r.lease))
	if err != nil {
		log.Warn().Str("room_id", room.RoomID).Str("target", room.RouteTarget).Str("task_id", task.ID).Err(err).Msg("matrix_pulse_claim_failed")
		return
	}
	if !claimed {
		return
	}

	plan := r.service.EvaluateRoom(now, room, []persistence.PulseTask{task}, room.RouteTarget)
	prompt := r.service.BuildPrompt(now, plan, r.interval)
	result, runErr := r.app.handlePulseRoom(ctx, room.RoomID, target, room.ProjectID, prompt)
	pulseErr := ""
	if runErr != nil {
		pulseErr = runErr.Error()
	}
	if err := r.store.CompleteRoomPulse(ctx, room.RoomID, room.RouteTarget, claimToken, time.Now().UTC(), result, pulseErr, []string{task.ID}); err != nil {
		log.Warn().Str("room_id", room.RoomID).Str("target", room.RouteTarget).Str("task_id", task.ID).Err(err).Msg("matrix_pulse_complete_failed")
	}
}
