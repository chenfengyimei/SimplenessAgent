export namespace app {

	export class PendingWrite {
	    task_id: string;
	    step_id: string;
	    path: string;
	    content: string;
	    expected_content_hash: string;

	    static createFrom(source: any = {}) {
	        return new PendingWrite(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.step_id = source["step_id"];
	        this.path = source["path"];
	        this.content = source["content"];
	        this.expected_content_hash = source["expected_content_hash"];
	    }
	}
	export class PendingWriteBatch {
	    task_id: string;
	    step_id: string;
	    writes: PendingWrite[];

	    static createFrom(source: any = {}) {
	        return new PendingWriteBatch(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.step_id = source["step_id"];
	        this.writes = this.convertValues(source["writes"], PendingWrite);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TaskSnapshot {
	    task: contracts.Task;
	    plan: contracts.PlanVersion;
	    steps: contracts.StepRuntime[];
	    events: contracts.EventEnvelope[];

	    static createFrom(source: any = {}) {
	        return new TaskSnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task = this.convertValues(source["task"], contracts.Task);
	        this.plan = this.convertValues(source["plan"], contracts.PlanVersion);
	        this.steps = this.convertValues(source["steps"], contracts.StepRuntime);
	        this.events = this.convertValues(source["events"], contracts.EventEnvelope);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace contracts {

	export class AcceptanceCriterion {
	    id: string;
	    type: string;
	    description: string;
	    spec: Record<string, any>;

	    static createFrom(source: any = {}) {
	        return new AcceptanceCriterion(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.description = source["description"];
	        this.spec = source["spec"];
	    }
	}
	export class CapabilitySnapshot {
	    capability_snapshot_id?: string;
	    deployment_id?: string;
	    version: number;
	    supports_tools: boolean;
	    supports_streaming: boolean;
	    supports_token_count: boolean;
	    reliable_context_tokens: number;
	    // Go type: time
	    probed_at: any;

	    static createFrom(source: any = {}) {
	        return new CapabilitySnapshot(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.capability_snapshot_id = source["capability_snapshot_id"];
	        this.deployment_id = source["deployment_id"];
	        this.version = source["version"];
	        this.supports_tools = source["supports_tools"];
	        this.supports_streaming = source["supports_streaming"];
	        this.supports_token_count = source["supports_token_count"];
	        this.reliable_context_tokens = source["reliable_context_tokens"];
	        this.probed_at = this.convertValues(source["probed_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Constraint {
	    id: string;
	    text: string;
	    hard: boolean;

	    static createFrom(source: any = {}) {
	        return new Constraint(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.text = source["text"];
	        this.hard = source["hard"];
	    }
	}
	export class ConversationMessage {
	    message_id: string;
	    conversation_id: string;
	    turn_task_id?: string;
	    role: string;
	    content: string;
	    // Go type: time
	    created_at: any;

	    static createFrom(source: any = {}) {
	        return new ConversationMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message_id = source["message_id"];
	        this.conversation_id = source["conversation_id"];
	        this.turn_task_id = source["turn_task_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Deployment {
	    deployment_id: string;
	    version: number;
	    name: string;
	    provider_type: string;
	    location: string;
	    endpoint: string;
	    credential_ref?: string;
	    model: string;
	    runtime?: string;
	    runtime_version?: string;
	    quantization?: string;
	    model_profile_id?: string;
	    capability_snapshot_id?: string;
	    enabled: boolean;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Deployment(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.deployment_id = source["deployment_id"];
	        this.version = source["version"];
	        this.name = source["name"];
	        this.provider_type = source["provider_type"];
	        this.location = source["location"];
	        this.endpoint = source["endpoint"];
	        this.credential_ref = source["credential_ref"];
	        this.model = source["model"];
	        this.runtime = source["runtime"];
	        this.runtime_version = source["runtime_version"];
	        this.quantization = source["quantization"];
	        this.model_profile_id = source["model_profile_id"];
	        this.capability_snapshot_id = source["capability_snapshot_id"];
	        this.enabled = source["enabled"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EventEnvelope {
	    event_id: string;
	    event_version: number;
	    event_type: string;
	    aggregate_type: string;
	    aggregate_id: string;
	    run_id?: string;
	    sequence: number;
	    // Go type: time
	    timestamp: any;
	    actor_type: string;
	    actor_id: string;
	    correlation_id?: string;
	    causation_id?: string;
	    payload: Record<string, any>;

	    static createFrom(source: any = {}) {
	        return new EventEnvelope(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.event_id = source["event_id"];
	        this.event_version = source["event_version"];
	        this.event_type = source["event_type"];
	        this.aggregate_type = source["aggregate_type"];
	        this.aggregate_id = source["aggregate_id"];
	        this.run_id = source["run_id"];
	        this.sequence = source["sequence"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.actor_type = source["actor_type"];
	        this.actor_id = source["actor_id"];
	        this.correlation_id = source["correlation_id"];
	        this.causation_id = source["causation_id"];
	        this.payload = source["payload"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExpectedOutput {
	    name: string;
	    type: string;
	    required: boolean;

	    static createFrom(source: any = {}) {
	        return new ExpectedOutput(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.type = source["type"];
	        this.required = source["required"];
	    }
	}
	export class StepBudget {
	    max_attempts: number;
	    max_iterations: number;
	    max_duration_ms: number;
	    max_input_tokens: number;
	    max_output_tokens: number;

	    static createFrom(source: any = {}) {
	        return new StepBudget(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_attempts = source["max_attempts"];
	        this.max_iterations = source["max_iterations"];
	        this.max_duration_ms = source["max_duration_ms"];
	        this.max_input_tokens = source["max_input_tokens"];
	        this.max_output_tokens = source["max_output_tokens"];
	    }
	}
	export class StepSpec {
	    version: number;
	    step_id: string;
	    title: string;
	    goal: string;
	    dependencies: string[];
	    allowed_tools: string[];
	    workspace_scopes: string[];
	    expected_outputs: ExpectedOutput[];
	    acceptance_criteria: AcceptanceCriterion[];
	    risk: string;
	    budget: StepBudget;
	    execution_mode: string;
	    preferred_role: string;

	    static createFrom(source: any = {}) {
	        return new StepSpec(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.step_id = source["step_id"];
	        this.title = source["title"];
	        this.goal = source["goal"];
	        this.dependencies = source["dependencies"];
	        this.allowed_tools = source["allowed_tools"];
	        this.workspace_scopes = source["workspace_scopes"];
	        this.expected_outputs = this.convertValues(source["expected_outputs"], ExpectedOutput);
	        this.acceptance_criteria = this.convertValues(source["acceptance_criteria"], AcceptanceCriterion);
	        this.risk = source["risk"];
	        this.budget = this.convertValues(source["budget"], StepBudget);
	        this.execution_mode = source["execution_mode"];
	        this.preferred_role = source["preferred_role"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PlanVersion {
	    version: number;
	    plan_id: string;
	    task_id: string;
	    revision: number;
	    parent_plan_id?: string;
	    reason: string;
	    summary: string;
	    steps: StepSpec[];
	    created_by_agent_id: string;
	    // Go type: time
	    created_at: any;

	    static createFrom(source: any = {}) {
	        return new PlanVersion(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.plan_id = source["plan_id"];
	        this.task_id = source["task_id"];
	        this.revision = source["revision"];
	        this.parent_plan_id = source["parent_plan_id"];
	        this.reason = source["reason"];
	        this.summary = source["summary"];
	        this.steps = this.convertValues(source["steps"], StepSpec);
	        this.created_by_agent_id = source["created_by_agent_id"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

	export class StepRuntime {
	    step_id: string;
	    plan_id: string;
	    status: string;
	    evidence_ids: string[];
	    artifact_ids: string[];
	    last_error_code?: string;

	    static createFrom(source: any = {}) {
	        return new StepRuntime(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.step_id = source["step_id"];
	        this.plan_id = source["plan_id"];
	        this.status = source["status"];
	        this.evidence_ids = source["evidence_ids"];
	        this.artifact_ids = source["artifact_ids"];
	        this.last_error_code = source["last_error_code"];
	    }
	}

	export class TaskBudget {
	    max_duration_ms: number;
	    max_steps: number;
	    max_replans: number;
	    max_model_input_tokens: number;
	    max_model_output_tokens: number;
	    max_cost?: number;

	    static createFrom(source: any = {}) {
	        return new TaskBudget(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_duration_ms = source["max_duration_ms"];
	        this.max_steps = source["max_steps"];
	        this.max_replans = source["max_replans"];
	        this.max_model_input_tokens = source["max_model_input_tokens"];
	        this.max_model_output_tokens = source["max_model_output_tokens"];
	        this.max_cost = source["max_cost"];
	    }
	}
	export class TaskSpec {
	    version: number;
	    task_id: string;
	    workspace_id: string;
	    title: string;
	    goal: string;
	    non_goals: string[];
	    constraints: Constraint[];
	    acceptance_criteria: AcceptanceCriterion[];
	    assumptions: string[];
	    deployment_id: string;
	    permission_profile_id: string;
	    budget: TaskBudget;
	    allow_subagents: boolean;
	    // Go type: time
	    created_at: any;

	    static createFrom(source: any = {}) {
	        return new TaskSpec(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.task_id = source["task_id"];
	        this.workspace_id = source["workspace_id"];
	        this.title = source["title"];
	        this.goal = source["goal"];
	        this.non_goals = source["non_goals"];
	        this.constraints = this.convertValues(source["constraints"], Constraint);
	        this.acceptance_criteria = this.convertValues(source["acceptance_criteria"], AcceptanceCriterion);
	        this.assumptions = source["assumptions"];
	        this.deployment_id = source["deployment_id"];
	        this.permission_profile_id = source["permission_profile_id"];
	        this.budget = this.convertValues(source["budget"], TaskBudget);
	        this.allow_subagents = source["allow_subagents"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Task {
	    id: string;
	    version: number;
	    workspace_id: string;
	    conversation_id?: string;
	    title: string;
	    goal: string;
	    status: string;
	    spec: TaskSpec;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Task(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.workspace_id = source["workspace_id"];
	        this.conversation_id = source["conversation_id"];
	        this.title = source["title"];
	        this.goal = source["goal"];
	        this.status = source["status"];
	        this.spec = this.convertValues(source["spec"], TaskSpec);
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

	export class Workspace {
	    id: string;
	    version: number;
	    name: string;
	    root_path: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;

	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.version = source["version"];
	        this.name = source["name"];
	        this.root_path = source["root_path"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace diagnostics {

	export class Entry {
	    // Go type: time
	    timestamp: any;
	    level: string;
	    component: string;
	    message: string;
	    fields?: Record<string, string>;

	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.level = source["level"];
	        this.component = source["component"];
	        this.message = source["message"];
	        this.fields = source["fields"];
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace main {

	export class TurnReportView {
	    summary: string;
	    tool_name: string;
	    files: string[];
	    truncated: boolean;
	    pending_write?: app.PendingWriteBatch;

	    static createFrom(source: any = {}) {
	        return new TurnReportView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.tool_name = source["tool_name"];
	        this.files = source["files"];
	        this.truncated = source["truncated"];
	        this.pending_write = this.convertValues(source["pending_write"], app.PendingWriteBatch);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConversationTurn {
	    snapshot: app.TaskSnapshot;
	    report: TurnReportView;

	    static createFrom(source: any = {}) {
	        return new ConversationTurn(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], app.TaskSnapshot);
	        this.report = this.convertValues(source["report"], TurnReportView);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ConversationView {
	    conversation: contracts.Task;
	    messages: contracts.ConversationMessage[];
	    turns: ConversationTurn[];

	    static createFrom(source: any = {}) {
	        return new ConversationView(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation = this.convertValues(source["conversation"], contracts.Task);
	        this.messages = this.convertValues(source["messages"], contracts.ConversationMessage);
	        this.turns = this.convertValues(source["turns"], ConversationTurn);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
