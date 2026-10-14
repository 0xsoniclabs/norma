// Copyright 2024 Fantom Foundation
// This file is part of Norma System Testing Infrastructure for Sonic.
//
// Norma is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// Norma is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with Norma. If not, see <http://www.gnu.org/licenses/>.

package apps

import (
	"errors"
	"testing"

	"github.com/0xsoniclabs/norma/driver"
	"github.com/0xsoniclabs/norma/driver/steps"
	"github.com/0xsoniclabs/norma/load/shaper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func validRunApp() *RunApp {
	rate := float32(10)
	return &RunApp{
		App:  steps.App{App: "load"},
		Type: "counter",
		Rate: &shaper.Rate{Constant: &rate},
	}
}

func TestRunApp_Validate(t *testing.T) {
	negative, zero, rate := float32(-1), 0, float32(5)
	tests := map[string]struct {
		edit      func(*RunApp)
		wantError string
	}{
		"valid":            {edit: func(*RunApp) {}},
		"missing name":     {edit: func(s *RunApp) { s.App.App = "" }, wantError: "requires an app identifier"},
		"name with spaces": {edit: func(s *RunApp) { s.App.App = "my app" }, wantError: "app name must match"},
		"missing type":     {edit: func(s *RunApp) { s.Type = "" }, wantError: "requires a type"},
		"unknown type":     {edit: func(s *RunApp) { s.Type = "bogus" }, wantError: "unknown application type"},
		"missing rate":     {edit: func(s *RunApp) { s.Rate = nil }, wantError: "requires a rate"},
		"negative rate": {
			edit:      func(s *RunApp) { s.Rate = &shaper.Rate{Constant: &negative} },
			wantError: "must be >= 0",
		},
		"two load shapes": {
			edit:      func(s *RunApp) { s.Rate.Wave = &shaper.Wave{Max: rate, Period: 1} },
			wantError: "exactly one load shape",
		},
		"no users": {edit: func(s *RunApp) { s.Users = &zero }, wantError: "number of users must be >= 1"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			step := validRunApp()
			test.edit(step)
			err := step.Validate()
			if test.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.wantError)
			}
		})
	}
}

func TestRunApp_StartsAndTracksTheApplication(t *testing.T) {
	ctrl := gomock.NewController(t)
	net := driver.NewMockNetwork(ctrl)
	app := driver.NewMockApplication(ctrl)
	step := validRunApp()

	gomock.InOrder(
		net.EXPECT().CreateApplication(gomock.Any(), &driver.ApplicationConfig{
			Name:  "load",
			Type:  "counter",
			Rate:  step.Rate,
			Users: 1, // the default when users is omitted
		}).Return(app, nil),
		app.EXPECT().Start(gomock.Any()).Return(nil),
	)

	env := steps.NewEnv(net, nil, nil)
	require.NoError(t, step.Run(t.Context(), env))
	require.Equal(t, app, env.Apps["load"])
}

// An application that did not start is not tracked, so a later stopApp
// reports it as unknown instead of stopping something that never ran.
func TestRunApp_DoesNotTrackAnApplicationThatFailedToStart(t *testing.T) {
	tests := map[string]func(*driver.MockNetwork, *driver.MockApplication){
		"creation fails": func(net *driver.MockNetwork, _ *driver.MockApplication) {
			net.EXPECT().CreateApplication(gomock.Any(), gomock.Any()).Return(nil, errors.New("no image"))
		},
		"start fails": func(net *driver.MockNetwork, app *driver.MockApplication) {
			net.EXPECT().CreateApplication(gomock.Any(), gomock.Any()).Return(app, nil)
			app.EXPECT().Start(gomock.Any()).Return(errors.New("no funds"))
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			net := driver.NewMockNetwork(ctrl)
			setup(net, driver.NewMockApplication(ctrl))

			env := steps.NewEnv(net, nil, nil)
			require.ErrorContains(t, validRunApp().Run(t.Context(), env), "application load")
			require.Empty(t, env.Apps)
		})
	}
}

func TestStopApp_StopsAndForgetsTheApplication(t *testing.T) {
	app := driver.NewMockApplication(gomock.NewController(t))
	app.EXPECT().Stop().Return(nil)

	env := steps.NewEnv(nil, nil, nil)
	env.Apps["load"] = app
	require.NoError(t, (&StopApp{steps.App{App: "load"}}).Run(t.Context(), env))
	require.Empty(t, env.Apps)
}

func TestStopApp_ReportsAnUnknownApplication(t *testing.T) {
	err := (&StopApp{steps.App{App: "ghost"}}).Run(t.Context(), steps.NewEnv(nil, nil, nil))
	require.ErrorContains(t, err, `application "ghost" not found`)
}
