import React, { useState, useEffect } from 'react';
import {
  MantineProvider,
  Container,
  Tabs,
  Badge,
  Alert,
  Loader,
  Group,
  createTheme,
} from '@mantine/core';
import { Notifications, notifications } from '@mantine/notifications';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import {
  IconAlertCircle,
  IconScale,
  IconBuildingBank,
} from '@tabler/icons-react';

import '@mantine/core/styles.css';
import '@mantine/notifications/styles.css';

import { Exception, UserRole, WorkflowDetail } from './types';
import { api, getActiveRole } from './api/client';
import { Header } from './components/Header';
import { DevToolsBar } from './components/DevToolsBar';
import { SummaryCards } from './components/SummaryCards';
import { ExceptionTable } from './components/ExceptionTable';
import { SettlementsTable } from './components/SettlementsTable';
import { InvestigationDrawer } from './components/InvestigationDrawer';
import { AuditDrawer } from './components/AuditDrawer';
import { EmptyStateBanner } from './components/EmptyStateBanner';

const theme = createTheme({
  primaryColor: 'indigo',
  fontFamily: 'Inter, sans-serif',
  fontFamilyMonospace: 'JetBrains Mono, monospace',
});

export function App() {
  const queryClient = useQueryClient();

  const [activeRole, setActiveRole] = useState<UserRole>(getActiveRole());
  const [syncInvestigation, setSyncInvestigation] = useState<boolean>(true);
  const [investigatingId, setInvestigatingId] = useState<number | null>(null);

  // Selected Exception for Investigation Drawer
  const [selectedException, setSelectedException] = useState<Exception | null>(null);
  const [drawerOpened, setDrawerOpened] = useState<boolean>(false);

  // Audit Drawer state
  const [auditDrawerOpened, setAuditDrawerOpened] = useState<boolean>(false);

  // Active Tab
  const [activeTab, setActiveTab] = useState<string | null>('dashboard');

  // Liveness Check
  const { data: health } = useQuery({
    queryKey: ['health'],
    queryFn: api.getHealth,
    refetchInterval: 10000,
  });

  // KPI Summary
  const {
    data: summary,
    isLoading: isLoadingSummary,
    error: summaryError,
  } = useQuery({
    queryKey: ['summary', activeRole],
    queryFn: api.getSummary,
    refetchInterval: 5000,
  });

  // Exceptions List
  const {
    data: exceptions = [],
    isLoading: isLoadingExceptions,
    error: exceptionsError,
  } = useQuery({
    queryKey: ['exceptions', activeRole],
    queryFn: api.getExceptions,
    refetchInterval: 5000,
  });

  // Settlement Batches List
  const { data: settlements = [] } = useQuery({
    queryKey: ['settlements', activeRole],
    queryFn: api.getSettlements,
    refetchInterval: 10000,
  });

  // Selected Exception Detail (refreshed when drawer opens)
  const { data: freshSelectedException } = useQuery({
    queryKey: ['exception', selectedException?.id, activeRole],
    queryFn: () => (selectedException ? api.getException(selectedException.id) : null),
    enabled: !!selectedException && drawerOpened,
    refetchInterval: 3000,
  });

  const activeException = freshSelectedException || selectedException;

  // Workflow Details for active exception
  const workflowId = activeException?.latest_workflow_id;
  const { data: workflowDetail, isLoading: isLoadingWorkflow } = useQuery({
    queryKey: ['workflow', workflowId, activeRole],
    queryFn: () => (workflowId ? api.getWorkflow(workflowId) : null),
    enabled: !!workflowId && drawerOpened,
    refetchInterval: (query) => {
      const isRunning = query.state.data?.workflow?.status === 'running';
      return isRunning ? 2000 : 5000;
    },
  });

  // Audit Events
  const { data: auditEvents = [], isLoading: isLoadingAudit } = useQuery({
    queryKey: ['audit-events', activeRole],
    queryFn: api.getAuditEvents,
    enabled: auditDrawerOpened,
    refetchInterval: 5000,
  });

  // Sync role changes
  useEffect(() => {
    const handleRoleChanged = () => {
      setActiveRole(getActiveRole());
      queryClient.invalidateQueries();
    };
    window.addEventListener('role_changed', handleRoleChanged);
    return () => window.removeEventListener('role_changed', handleRoleChanged);
  }, [queryClient]);

  // Mutations
  const loadSeedMutation = useMutation({
    mutationFn: api.importSeed,
    onSuccess: (data) => {
      notifications.show({
        title: 'Seed Data Loaded',
        message: `Successfully loaded seed data (${data.message || 'all files processed'})`,
        color: 'teal',
      });
      queryClient.invalidateQueries();
    },
    onError: (err: Error) => {
      notifications.show({
        title: 'Seed Load Failed',
        message: err.message,
        color: 'red',
      });
    },
  });

  const runReconMutation = useMutation({
    mutationFn: (asOf: string) => api.runReconciliation(asOf),
    onSuccess: (data) => {
      notifications.show({
        title: 'Reconciliation Run Completed',
        message: `Reconciled ${data.batches_processed || 6} batches. ${data.exceptions_upsert || 4} open exceptions.`,
        color: 'indigo',
      });
      queryClient.invalidateQueries();
    },
    onError: (err: Error) => {
      notifications.show({
        title: 'Reconciliation Failed',
        message: err.message,
        color: 'red',
      });
    },
  });

  const investigateMutation = useMutation({
    mutationFn: ({ id, sync }: { id: number; sync: boolean }) =>
      api.investigateException(id, sync),
    onMutate: ({ id }) => {
      setInvestigatingId(id);
    },
    onSuccess: (data, { id }) => {
      setInvestigatingId(null);
      notifications.show({
        title: 'Investigation Finished',
        message: `Agent completed analysis. Outcome: ${data.workflow?.conclusion?.conclusion || data.status}`,
        color: 'teal',
      });
      queryClient.invalidateQueries();

      // Open drawer automatically for inspection
      const exc = exceptions.find((e) => e.id === id);
      if (exc) {
        setSelectedException({
          ...exc,
          latest_workflow_id: data.workflow?.id || exc.latest_workflow_id,
        });
        setDrawerOpened(true);
      }
    },
    onError: (err: Error) => {
      setInvestigatingId(null);
      notifications.show({
        title: 'Investigation Error',
        message: err.message,
        color: 'red',
      });
    },
  });

  const escalateMutation = useMutation({
    mutationFn: (id: number) => api.escalateException(id),
    onSuccess: () => {
      notifications.show({
        title: 'Exception Escalated',
        message: 'Escalated to Treasury Operations for manual review.',
        color: 'violet',
      });
      queryClient.invalidateQueries();
    },
    onError: (err: Error) => {
      notifications.show({
        title: 'Escalation Failed',
        message: err.message,
        color: 'red',
      });
    },
  });

  const approveRemediationMutation = useMutation({
    mutationFn: (id: number) => api.approveRemediation(id),
    onSuccess: () => {
      notifications.show({
        title: 'Remediation Approved & Executed',
        message: 'Remediation executed against external system and logged in audit trail.',
        color: 'teal',
      });
      queryClient.invalidateQueries();
    },
    onError: (err: Error) => {
      notifications.show({
        title: 'Approval Failed',
        message: err.message,
        color: 'red',
      });
    },
  });

  const rejectRemediationMutation = useMutation({
    mutationFn: ({ id, note }: { id: number; note?: string }) =>
      api.rejectRemediation(id, note),
    onSuccess: () => {
      notifications.show({
        title: 'Remediation Rejected',
        message: 'Proposal was rejected and closed.',
        color: 'gray',
      });
      queryClient.invalidateQueries();
    },
    onError: (err: Error) => {
      notifications.show({
        title: 'Rejection Failed',
        message: err.message,
        color: 'red',
      });
    },
  });

  const retryRemediationMutation = useMutation({
    mutationFn: (id: number) => api.executeRemediation(id),
    onSuccess: () => {
      notifications.show({
        title: 'Remediation Executed on Retry',
        message: 'Idempotent execution succeeded.',
        color: 'teal',
      });
      queryClient.invalidateQueries();
    },
    onError: (err: Error) => {
      notifications.show({
        title: 'Retry Failed',
        message: err.message,
        color: 'red',
      });
    },
  });

  const isEmptyDB = !summary || (summary.batches_total === 0 && summary.payment_volume_minor === 0);

  return (
    <MantineProvider theme={theme}>
      <Notifications position="top-right" />
      <Container size="xl" py="lg">
        {/* Top Header */}
        <Header
          onLoadSeed={() => loadSeedMutation.mutate()}
          isLoadingSeed={loadSeedMutation.isPending}
          onRunRecon={(asOf) => runReconMutation.mutate(asOf)}
          isRunningRecon={runReconMutation.isPending}
          onOpenAudit={() => setAuditDrawerOpened(true)}
          onRoleChange={(role) => setActiveRole(role)}
          systemHealth={health || null}
        />

        {/* Dev Tools & Simulation Bar */}
        <DevToolsBar
          currentRole={activeRole}
          onRoleChange={setActiveRole}
          syncInvestigation={syncInvestigation}
          onSyncChange={setSyncInvestigation}
        />

        {/* API Error Banner if any */}
        {(summaryError || exceptionsError) && (
          <Alert
            icon={<IconAlertCircle size={16} />}
            title="Backend Communication Error"
            color="red"
            variant="filled"
            mb="md"
          >
            {summaryError?.message || exceptionsError?.message}
          </Alert>
        )}

        {/* KPI Summary Cards */}
        <SummaryCards summary={summary || null} isLoading={isLoadingSummary} />

        {/* Empty State Banner if no seed data */}
        {isEmptyDB && !isLoadingSummary && (
          <EmptyStateBanner
            onLoadSeed={() => loadSeedMutation.mutate()}
            isLoading={loadSeedMutation.isPending}
            currentRole={activeRole}
          />
        )}

        {/* Navigation Tabs */}
        <Tabs value={activeTab} onChange={setActiveTab} mb="md">
          <Tabs.List>
            <Tabs.Tab
              value="dashboard"
              leftSection={<IconScale size={16} />}
              rightSection={
                exceptions.length > 0 ? (
                  <Badge size="xs" color="red">
                    {exceptions.length}
                  </Badge>
                ) : null
              }
            >
              Reconciliation Exceptions
            </Tabs.Tab>
            <Tabs.Tab
              value="settlements"
              leftSection={<IconBuildingBank size={16} />}
              rightSection={
                settlements.length > 0 ? (
                  <Badge size="xs" color="gray">
                    {settlements.length}
                  </Badge>
                ) : null
              }
            >
              Settlement Batches
            </Tabs.Tab>
          </Tabs.List>

          <Tabs.Panel value="dashboard" pt="md">
            <ExceptionTable
              exceptions={exceptions}
              onInvestigate={(id) =>
                investigateMutation.mutate({ id, sync: syncInvestigation })
              }
              investigatingId={investigatingId}
              onSelectException={(exc) => {
                setSelectedException(exc);
                setDrawerOpened(true);
              }}
              onEscalate={(id) => escalateMutation.mutate(id)}
              currentRole={activeRole}
            />
          </Tabs.Panel>

          <Tabs.Panel value="settlements" pt="md">
            <SettlementsTable batches={settlements} />
          </Tabs.Panel>
        </Tabs>

        {/* Split-View Investigation Drawer */}
        <InvestigationDrawer
          opened={drawerOpened}
          onClose={() => setDrawerOpened(false)}
          exception={activeException}
          workflowDetail={workflowDetail || null}
          isLoadingWorkflow={isLoadingWorkflow}
          remediation={activeException?.remediation}
          onApproveRemediation={(id) => approveRemediationMutation.mutate(id)}
          isApproving={approveRemediationMutation.isPending}
          onRejectRemediation={(id, note) =>
            rejectRemediationMutation.mutate({ id, note })
          }
          isRejecting={rejectRemediationMutation.isPending}
          onRetryRemediation={(id) => retryRemediationMutation.mutate(id)}
          isRetrying={retryRemediationMutation.isPending}
          currentRole={activeRole}
        />

        {/* Audit Log Drawer */}
        <AuditDrawer
          opened={auditDrawerOpened}
          onClose={() => setAuditDrawerOpened(false)}
          events={auditEvents}
          isLoading={isLoadingAudit}
        />
      </Container>
    </MantineProvider>
  );
}

