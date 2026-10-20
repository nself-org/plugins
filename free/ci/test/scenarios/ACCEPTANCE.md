# Scheduler acceptance matrix (P7-SCHED-15)

Maps every test of this suite (`free/ci/test/scenarios`, build tag `scenarios`; the `TestPlanted*` rows need `scenarios planted`)
to the Epic SCHED acceptance item it proves. `TestAcceptanceMatrixMapsEveryTest` fails when a test is missing from this table, a row
names a test that does not exist, an item falls outside 2-6, or an item has no test.

| Item | Epic acceptance |
|---|---|
| 2 | Scenario 4, heterogeneous matrix (three entries, matching runners, `platform.none_available`, E618, native legs) |
| 3 | Scenario 6, hybrid (local, two LAN fakes, hosted fake; `balanced` and `preserve-hosted-quota`; D4 fixture orders; explain output) |
| 4 | Scenario 10, capacity saturation (no reservation or slot breach, fairness bound, E694, interactive beats background) |
| 5 | Economics (approval, budgets, reservation under concurrency, spend keys, included margin, GitHub never metered, ledger, clock step back) |
| 6 | Burst (provision, join, run, teardown; kill at each step; 2 h outage; sweeper limits; one job per VM; window cap; lifetime reservation) |

| Test | Acceptance item | What it proves |
|---|---|---|
| `TestAcceptanceMatrixMapsEveryTest` | 2, 3, 4, 5, 6 | Matrix completeness |
| `TestBurstCoordinatorKillLeavesNoLabelledResourceAfterOneSweep` | 6 | Burst |
| `TestBurstFullLifecycleLeavesNothingBehind` | 6 | Burst |
| `TestBurstIdleVmIsTornDownAtIdleTimeout` | 6 | Burst |
| `TestBurstNoProvisionWithoutADayOrMonthBudget` | 5, 6 | Burst: no cloud call without paid_enabled and a budget |
| `TestBurstNoProvisionWithoutPaidEnabled` | 5, 6 | Burst: no cloud call without paid_enabled and a budget |
| `TestBurstOneJobPerVmAndCertRevokedAtTeardown` | 6 | Burst |
| `TestBurstOneQueuedJobCausesAtMostOneProvisionPerWindow` | 6 | Burst |
| `TestBurstPersonalNeverProvisionsCloudCapacity` | 6 | Burst |
| `TestBurstPrivateInfrastructureNeverReachesABurstVMByDefault` | 6 | Burst |
| `TestBurstReservationCoversTheWholeLifetimeNotJustOneJob` | 6 | Burst |
| `TestBurstRetriedTeardownDoesNotDoubleChargeTheLedger` | 6 | Burst |
| `TestBurstSweeperNeverTouchesForeignUnlabelledOrExcluded` | 6 | Burst |
| `TestBurstTeardownFailedTwoHoursIsDestroyedByFirstSweepAfterRecovery` | 6 | Burst |
| `TestEconomicsApprovalPausesUntilAnAdminApproves` | 5 | Economics |
| `TestEconomicsBudgetExceededIsIneligibleAtEveryScope` | 5 | Economics |
| `TestEconomicsClockStepBackNeverChargesAnEarlierWindow` | 5 | Economics |
| `TestEconomicsConcurrentSubmissionsNeverExceedTheBudget` | 5 | Economics |
| `TestEconomicsGitHubHostedIsNeverMetered` | 5 | Economics |
| `TestEconomicsIncludedAdmissionStopsAtTheMargin` | 5 | Economics |
| `TestEconomicsIncludedStopsAtTheMargin` | 5 | Economics |
| `TestEconomicsLedgerLabelsEstimatedComputedKnownAndUnknown` | 5 | Economics |
| `TestEconomicsNoPaidAdmissionWithoutPaidEnabledAndABudget` | 5 | Economics |
| `TestEconomicsNoPaidPlacementWithoutPaidEnabledAndABudget` | 5 | Economics |
| `TestEconomicsRevisionBudgetEnablesNothing` | 5 | Economics |
| `TestEconomicsSpendKeysOnProtectedBranchChangeNothing` | 5 | Economics |
| `TestPlantedGitHubMetered` | 5 | Planted: GitHub provider priced as metered |
| `TestPlantedLeaseOutsideAdmit` | 4 | Planted: lease created outside Admit |
| `TestPlantedPriorityBypassesEligibility` | 4 | Planted: priority bypassing eligibility |
| `TestPlantedProvisionWithoutBudget` | 5 | Planted: placement of paid capacity without budget |
| `TestPlantedProvisionWithoutSpend` | 5, 6 | Planted: cloud provision without paid_enabled and a budget |
| `TestPlantedProvisioningStorm` | 6 | Planted: provisioning storm |
| `TestPlantedReservationOvercommit` | 4 | Planted: reservation over-commit |
| `TestPlantedRevisionBudgetEnablesSpend` | 5 | Planted: revision-set budget enabling spend |
| `TestPlantedSweeperDeletesForeign` | 6 | Planted: sweeper deleting a foreign resource |
| `TestPolicyPaidEnabledAdminAudited` | 5 | Economics |
| `TestPolicyPaidEnabledPersonalOperator` | 5 | Economics |
| `TestPolicyPaidEnabledTeamMemberRefused` | 5 | Economics |
| `TestScenario10BackpressureCarriesExpectedWait` | 4 | Scenario 10, capacity saturation |
| `TestScenario10EveryLeaseComesFromAdmit` | 4 | Scenario 10, capacity saturation |
| `TestScenario10InteractiveStartsBeforeQueuedBackground` | 4 | Scenario 10, capacity saturation |
| `TestScenario10PriorityNeverAdmitsIneligible` | 4 | Scenario 10, capacity saturation |
| `TestScenario10SaturationStaysWithinCapacityAndFair` | 4 | Scenario 10, capacity saturation |
| `TestScenario4EachEntryLandsOnAMatchingRunner` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario4MatrixExpandsToThreeEntries` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario4MissingPlatformFailsAFreshForegroundRun` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario4MissingPlatformWaitsOnACoordinator` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario4Native` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario4RealWorldDispatch` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario4RealWorldPlacesEachEntryOnItsPlatform` | 2 | Scenario 4, heterogeneous matrix |
| `TestScenario6DispatcherMovesOffHosted` | 3 | Scenario 6, hybrid |
| `TestScenario6PlacementsFollowTheFixtureOrder` | 3 | Scenario 6, hybrid |
| `TestScenario6PreserveHostedQuotaMovesWorkOffHosted` | 3 | Scenario 6, hybrid |
| `TestScenario6ResultIsDeterministic` | 3 | Scenario 6, hybrid |
