// SPDX-License-Identifier: ice License 1.0

package verificationscenarios

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ice-blockchain/eskimo/users"
	"github.com/ice-blockchain/santa/tasks"
)

func TestGetPendingKYCVerificationScenarios(t *testing.T) {
	tests := []struct {
		name                           string
		completedSantaTasks            []string
		distributionScenariosCompleted []string
		expectedPendingScenarios       []string
	}{
		{
			name: "All tasks and scenarios completed",
			completedSantaTasks: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx", "signup_tokero",
				"join_twitter", "join_telegram", "join_bullish_cmc", "join_ion_cmc", "join_watchlist_cmc",
			},
			distributionScenariosCompleted: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx", "signup_tokero",
				"join_twitter", "join_telegram", "join_cmc",
			},
			expectedPendingScenarios: nil,
		},
		{
			name: "Some tasks and scenarios completed, but sauces task copmpleted, distribution scenario not completed",
			completedSantaTasks: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces",
			},
			distributionScenariosCompleted: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent",
			},
			expectedPendingScenarios: []string{
				"signup_sauces",
			},
		},
		{
			name:                "No completed santa tasks, but all scenarios completed",
			completedSantaTasks: []string{},
			distributionScenariosCompleted: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx",
				"join_twitter", "join_telegram", "join_cmc",
			},
			expectedPendingScenarios: nil,
		},
		{
			name: "Has all completed santa tasks and no scenarios completed",
			completedSantaTasks: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx",
				"join_twitter", "join_telegram", "join_bullish_cmc", "join_ion_cmc", "join_watchlist_cmc",
			},
			distributionScenariosCompleted: []string{},
			expectedPendingScenarios: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx",
				"join_twitter", "join_telegram", "join_cmc",
			},
		},
		{
			name: "Has other santa completed tasks that are not related to distribution scenarios",
			completedSantaTasks: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx",
				"join_twitter", "join_telegram", "join_bullish_cmc", "join_ion_cmc", "join_watchlist_cmc",
				"join_reddit_ion", "join_instagram_ion", "join_youtube", "join_portfolio_coingecko",
			},
			distributionScenariosCompleted: []string{},
			expectedPendingScenarios: []string{
				"signup_sunwaves", "signup_sealsend", "signup_callfluent", "signup_sauces", "signup_doctorx",
				"join_twitter", "join_telegram", "join_cmc",
			},
		},
		{
			name: "CMC",
			completedSantaTasks: []string{
				"join_bullish_cmc", "join_ion_cmc", "join_watchlist_cmc",
			},
			distributionScenariosCompleted: []string{},
			expectedPendingScenarios:       []string{"join_cmc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enums := make(users.Enum[string], 0)
			if tt.distributionScenariosCompleted != nil {
				for _, scenario := range tt.distributionScenariosCompleted {
					enums = append(enums, scenario)
				}
			}
			usr := &users.User{
				DistributionScenariosCompleted: &enums,
			}
			tsks := make([]*tasks.Task, len(tt.completedSantaTasks))
			for i, taskType := range tt.completedSantaTasks {
				tsks[i] = &tasks.Task{
					Type: tasks.Type(taskType),
				}
			}
			repo := &repository{}
			pendingScenarios := repo.getPendingScenarios(usr, tsks)
			require.NotNil(t, pendingScenarios)
			require.Len(t, pendingScenarios, len(tt.expectedPendingScenarios))
			for _, expectedScenario := range tt.expectedPendingScenarios {
				require.True(t, isScenarioPending(pendingScenarios, expectedScenario))
			}
		})
	}
}

func TestIsScenarioPending(t *testing.T) {
	cmcScenario := CoinDistributionScenarioCmc
	signUpTenantsScenario := CoinDistributionScenarioSignUpTenants
	tests := []struct {
		name              string
		pendingScenarios  []*Scenario
		scenario          Scenario
		expectedIsPending bool
	}{
		{
			name:              "Scenario is pending",
			pendingScenarios:  []*Scenario{&cmcScenario, &signUpTenantsScenario},
			scenario:          CoinDistributionScenarioCmc,
			expectedIsPending: true,
		},
		{
			name:              "Scenario is not pending",
			pendingScenarios:  []*Scenario{&cmcScenario},
			scenario:          CoinDistributionScenarioTelegram,
			expectedIsPending: false,
		},
		{
			name:              "Scenario is CoinDistributionScenarioSignUpTenants, we return true to check tenants tokens later",
			pendingScenarios:  []*Scenario{&cmcScenario},
			scenario:          CoinDistributionScenarioSignUpTenants,
			expectedIsPending: true,
		},
		{
			name:              "No pending scenarios",
			pendingScenarios:  []*Scenario{},
			scenario:          CoinDistributionScenarioCmc,
			expectedIsPending: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isPending := isScenarioPending(tt.pendingScenarios, string(tt.scenario))
			require.Equal(t, tt.expectedIsPending, isPending)
		})
	}
}
