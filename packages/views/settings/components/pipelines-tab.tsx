"use client";

import { useEffect, useState } from "react";
import { Plus, Save, Trash2 } from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import { Input } from "@multica/ui/components/ui/input";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { useWorkspaceId } from "@multica/core/hooks";
import type { Pipeline, PipelineStageRequest } from "@multica/core/types";
import { pipelineListOptions, useCreatePipeline, useDeletePipeline, useUpdatePipeline } from "@multica/core/pipelines";

type StageForm = PipelineStageRequest & { stage_instructions: string };
type PipelineForm = { name: string; description: string; stages: StageForm[] };

const EMPTY_PIPELINES: Pipeline[] = [];
const blankStage = (position: number): StageForm => ({ name: "", status: "", agent_id: "", stage_instructions: "", position });
const blankPipeline = (): PipelineForm => ({ name: "", description: "", stages: [blankStage(1)] });
const countLabel = (count: number) => `${count} stage${count === 1 ? "" : "s"}`;

function toForm(pipeline: Pipeline): PipelineForm {
  return {
    name: pipeline.name,
    description: pipeline.description ?? "",
    stages: pipeline.stages.length
      ? pipeline.stages.map((stage) => ({
          name: stage.name,
          status: stage.status,
          agent_id: stage.agent_id,
          stage_instructions: stage.stage_instructions ?? "",
          position: stage.position,
        }))
      : [blankStage(1)],
  };
}

const toStages = (stages: StageForm[]) =>
  stages.map((stage) => ({
    name: stage.name.trim(),
    status: stage.status.trim(),
    agent_id: stage.agent_id.trim(),
    stage_instructions: stage.stage_instructions.trim() || undefined,
    position: Number(stage.position),
  }));

const hasValidStages = (stages: PipelineStageRequest[]) =>
  stages.length > 0 && stages.every((stage) =>
    stage.name.trim().length > 0
    && stage.status.trim().length > 0
    && stage.agent_id.trim().length > 0
    && Number.isInteger(stage.position)
    && stage.position > 0,
  );

const toCreateRequest = (form: PipelineForm) => ({
  name: form.name.trim(),
  description: form.description.trim() || undefined,
  stages: toStages(form.stages),
});

const toUpdateRequest = (form: PipelineForm) => ({
  name: form.name.trim(),
  description: form.description.trim() || null,
  stages: toStages(form.stages),
});

export function PipelinesTab() {
  const wsId = useWorkspaceId();
  const { data: pipelineData, isLoading, isError } = useQuery(pipelineListOptions(wsId));
  const pipelines = pipelineData ?? EMPTY_PIPELINES;
  const createPipeline = useCreatePipeline();
  const updatePipeline = useUpdatePipeline();
  const deletePipeline = useDeletePipeline();
  const [drafts, setDrafts] = useState<Record<string, PipelineForm>>({});
  const [newPipeline, setNewPipeline] = useState<PipelineForm>(blankPipeline());

  useEffect(() => {
    setDrafts((current) => {
      const next: Record<string, PipelineForm> = {};
      for (const pipeline of pipelines) next[pipeline.id] = current[pipeline.id] ?? toForm(pipeline);
      return next;
    });
  }, [pipelines]);

  const patchDraft = (id: string, fallback: PipelineForm, update: (draft: PipelineForm) => PipelineForm) =>
    setDrafts((current) => ({ ...current, [id]: update(current[id] ?? fallback) }));

  const mutateCreate = () => {
    const payload = toCreateRequest(newPipeline);
    if (!payload.name || !hasValidStages(payload.stages)) {
      toast.error("Each stage needs a name, status, agent, and positive position");
      return;
    }
    createPipeline.mutate(payload, {
      onSuccess: () => {
        toast.success("Pipeline created");
        setNewPipeline(blankPipeline());
      },
      onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to create pipeline"),
    });
  };

  const mutateUpdate = (id: string, draft: PipelineForm) => {
    const payload = toUpdateRequest(draft);
    if (!payload.name || !payload.stages || !hasValidStages(payload.stages)) {
      toast.error("Each stage needs a name, status, agent, and positive position");
      return;
    }
    updatePipeline.mutate({ id, ...payload }, {
      onSuccess: () => toast.success("Pipeline updated"),
      onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to update pipeline"),
    });
  };

  const mutateDelete = (id: string) => {
    deletePipeline.mutate(id, {
      onSuccess: () => toast.success("Pipeline deleted"),
      onError: (error) => toast.error(error instanceof Error ? error.message : "Failed to delete pipeline"),
    });
  };

  if (isLoading) return <div>Loading pipelines...</div>;
  if (isError) return <div>Failed to load pipelines.</div>;

  return (
    <div className="space-y-8">
      <section className="space-y-4">
        <div>
          <h2 className="text-sm font-semibold">Pipelines</h2>
          <p className="text-xs text-muted-foreground">
            {pipelines.length} pipeline{pipelines.length === 1 ? "" : "s"} · {countLabel(pipelines.reduce((sum, pipeline) => sum + pipeline.stages.length, 0))} total
          </p>
        </div>

        <Card>
          <CardContent className="space-y-4">
            <div className="grid gap-3 md:grid-cols-2">
              <div className="space-y-1.5">
                <label className="text-xs font-medium">New pipeline name</label>
                <Input value={newPipeline.name} onChange={(e) => setNewPipeline((current) => ({ ...current, name: e.target.value }))} aria-label="New pipeline name" placeholder="Launch process" />
              </div>
              <div className="space-y-1.5 md:col-span-2">
                <label className="text-xs font-medium">New pipeline description</label>
                <Textarea value={newPipeline.description} onChange={(e) => setNewPipeline((current) => ({ ...current, description: e.target.value }))} aria-label="New pipeline description" placeholder="Optional description" />
              </div>
            </div>

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">New pipeline stages</h3>
                <Button type="button" variant="outline" size="sm" onClick={() => setNewPipeline((current) => ({ ...current, stages: [...current.stages, blankStage(current.stages.length + 1)] }))}>
                  <Plus className="h-3.5 w-3.5" />
                  Add stage
                </Button>
              </div>
              {newPipeline.stages.map((stage, index) => (
                <StageEditor key={`new-${index}`} prefix="New" stage={stage} onChange={(next) => setNewPipeline((current) => ({ ...current, stages: current.stages.map((item, i) => (i === index ? next : item)) }))} onRemove={newPipeline.stages.length > 1 ? () => setNewPipeline((current) => ({ ...current, stages: current.stages.filter((_, i) => i !== index).map((item, i) => ({ ...item, position: i + 1 })) })) : undefined} />
              ))}
            </div>

            <div className="flex justify-end">
              <Button type="button" onClick={mutateCreate} disabled={createPipeline.isPending}>
                <Save className="h-3.5 w-3.5" />
                Create pipeline
              </Button>
            </div>
          </CardContent>
        </Card>
      </section>

      <section className="space-y-4">
        {pipelines.map((pipeline) => {
          const draft = drafts[pipeline.id] ?? toForm(pipeline);
          return (
            <Card key={pipeline.id}>
              <CardContent className="space-y-4">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <h3 className="text-sm font-semibold">{pipeline.name}</h3>
                    <p className="text-xs text-muted-foreground">{pipeline.description ?? "No description"} · {countLabel(pipeline.stages.length)}</p>
                  </div>
                  <Button type="button" variant="ghost" size="icon" onClick={() => mutateDelete(pipeline.id)} disabled={deletePipeline.isPending} aria-label={`Delete ${pipeline.name}`}>
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>

                <div className="grid gap-3 md:grid-cols-2">
                  <div className="space-y-1.5">
                    <label className="text-xs font-medium">{pipeline.id} pipeline name</label>
                    <Input
                      value={draft.name}
                      onChange={(e) => patchDraft(pipeline.id, draft, (current) => ({ ...current, name: e.target.value }))}
                      aria-label={`${pipeline.id} pipeline name`}
                    />
                  </div>
                  <div className="space-y-1.5 md:col-span-2">
                    <label className="text-xs font-medium">{pipeline.id} pipeline description</label>
                    <Textarea value={draft.description} onChange={(e) => patchDraft(pipeline.id, draft, (current) => ({ ...current, description: e.target.value }))} aria-label={`${pipeline.id} pipeline description`} />
                  </div>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <h4 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">Stages</h4>
                    <Button type="button" variant="outline" size="sm" onClick={() => patchDraft(pipeline.id, draft, (current) => ({ ...current, stages: [...current.stages, blankStage(current.stages.length + 1)] }))}>
                      <Plus className="h-3.5 w-3.5" />
                      Add stage
                    </Button>
                  </div>
                  {draft.stages.map((stage, index) => (
                    <StageEditor key={`${pipeline.id}-${index}`} prefix={pipeline.id} stage={stage} onChange={(next) => patchDraft(pipeline.id, draft, (current) => ({ ...current, stages: current.stages.map((item, i) => (i === index ? next : item)) }))} onRemove={draft.stages.length > 1 ? () => patchDraft(pipeline.id, draft, (current) => ({ ...current, stages: current.stages.filter((_, i) => i !== index).map((item, i) => ({ ...item, position: i + 1 })) })) : undefined} />
                  ))}
                </div>

                <div className="flex justify-end">
                  <Button type="button" onClick={() => mutateUpdate(pipeline.id, draft)} disabled={updatePipeline.isPending}>
                    <Save className="h-3.5 w-3.5" />
                    Save
                  </Button>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </section>
    </div>
  );
}

function StageEditor({ prefix, stage, onChange, onRemove }: { prefix: string; stage: StageForm; onChange: (stage: StageForm) => void; onRemove?: () => void; }) {
  return (
    <div className="rounded-md border p-3 space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div className="grid flex-1 gap-3 md:grid-cols-2">
          <div className="space-y-1.5"><label className="text-xs font-medium">{prefix} stage {stage.position} name</label><Input value={stage.name} onChange={(e) => onChange({ ...stage, name: e.target.value })} aria-label={`${prefix} stage ${stage.position} name`} /></div>
          <div className="space-y-1.5"><label className="text-xs font-medium">{prefix} stage {stage.position} status</label><Input value={stage.status} onChange={(e) => onChange({ ...stage, status: e.target.value })} aria-label={`${prefix} stage ${stage.position} status`} /></div>
          <div className="space-y-1.5"><label className="text-xs font-medium">{prefix} stage {stage.position} agent</label><Input value={stage.agent_id} onChange={(e) => onChange({ ...stage, agent_id: e.target.value })} aria-label={`${prefix} stage ${stage.position} agent`} /></div>
          <div className="space-y-1.5"><label className="text-xs font-medium">{prefix} stage {stage.position} position</label><Input type="number" value={stage.position} onChange={(e) => onChange({ ...stage, position: Number(e.target.value) || 0 })} aria-label={`${prefix} stage ${stage.position} position`} /></div>
          <div className="md:col-span-2 space-y-1.5"><label className="text-xs font-medium">{prefix} stage {stage.position} instructions</label><Textarea value={stage.stage_instructions} onChange={(e) => onChange({ ...stage, stage_instructions: e.target.value })} aria-label={`${prefix} stage ${stage.position} instructions`} /></div>
        </div>
        {onRemove && <Button type="button" variant="ghost" size="icon" onClick={onRemove} aria-label={`Remove stage ${stage.position}`}><Trash2 className="h-4 w-4" /></Button>}
      </div>
    </div>
  );
}
