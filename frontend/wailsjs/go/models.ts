export namespace app {
	
	export class PostInput {
	    topicId: string;
	    body: string;
	    parentId?: string;
	    mentions: string[];
	    mode: string;
	    autoReveal: boolean;
	
	    static createFrom(source: any = {}) {
	        return new PostInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.topicId = source["topicId"];
	        this.body = source["body"];
	        this.parentId = source["parentId"];
	        this.mentions = source["mentions"];
	        this.mode = source["mode"];
	        this.autoReveal = source["autoReveal"];
	    }
	}
	export class PostResult {
	    message: domain.Message;
	    round?: domain.Round;
	
	    static createFrom(source: any = {}) {
	        return new PostResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.message = this.convertValues(source["message"], domain.Message);
	        this.round = this.convertValues(source["round"], domain.Round);
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
	export class Thread {
	    topic: domain.Topic;
	    messages: domain.Message[];
	    runs: domain.Run[];
	    rounds: domain.Round[];
	    participants: domain.Participant[];
	    context?: domain.ContextRevision;
	    lastEventSeq: number;
	
	    static createFrom(source: any = {}) {
	        return new Thread(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.topic = this.convertValues(source["topic"], domain.Topic);
	        this.messages = this.convertValues(source["messages"], domain.Message);
	        this.runs = this.convertValues(source["runs"], domain.Run);
	        this.rounds = this.convertValues(source["rounds"], domain.Round);
	        this.participants = this.convertValues(source["participants"], domain.Participant);
	        this.context = this.convertValues(source["context"], domain.ContextRevision);
	        this.lastEventSeq = source["lastEventSeq"];
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

export namespace domain {
	
	export class Reference {
	    label: string;
	    messageId?: string;
	    artifactId?: string;
	
	    static createFrom(source: any = {}) {
	        return new Reference(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.label = source["label"];
	        this.messageId = source["messageId"];
	        this.artifactId = source["artifactId"];
	    }
	}
	export class Decision {
	    text: string;
	    reason?: string;
	    messageId?: string;
	
	    static createFrom(source: any = {}) {
	        return new Decision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.reason = source["reason"];
	        this.messageId = source["messageId"];
	    }
	}
	export class ContextDoc {
	    goal: string;
	    constraints?: string[];
	    decisions?: Decision[];
	    done?: string[];
	    current?: string;
	    openQuestions?: string[];
	    references?: Reference[];
	
	    static createFrom(source: any = {}) {
	        return new ContextDoc(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goal = source["goal"];
	        this.constraints = source["constraints"];
	        this.decisions = this.convertValues(source["decisions"], Decision);
	        this.done = source["done"];
	        this.current = source["current"];
	        this.openQuestions = source["openQuestions"];
	        this.references = this.convertValues(source["references"], Reference);
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
	export class ContextRevision {
	    id: string;
	    topicId: string;
	    version: number;
	    status: string;
	    authorKind: string;
	    participantId?: string;
	    runId?: string;
	    doc: ContextDoc;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ContextRevision(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.topicId = source["topicId"];
	        this.version = source["version"];
	        this.status = source["status"];
	        this.authorKind = source["authorKind"];
	        this.participantId = source["participantId"];
	        this.runId = source["runId"];
	        this.doc = this.convertValues(source["doc"], ContextDoc);
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	
	export class Event {
	    seq: number;
	    type: string;
	    topicId?: string;
	    // Go type: time
	    at: any;
	    payload: number[];
	
	    static createFrom(source: any = {}) {
	        return new Event(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.seq = source["seq"];
	        this.type = source["type"];
	        this.topicId = source["topicId"];
	        this.at = this.convertValues(source["at"], null);
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
	export class FrozenInputs {
	    contextRevisionId?: string;
	    messageIds: string[];
	    artifactIds?: string[];
	
	    static createFrom(source: any = {}) {
	        return new FrozenInputs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.contextRevisionId = source["contextRevisionId"];
	        this.messageIds = source["messageIds"];
	        this.artifactIds = source["artifactIds"];
	    }
	}
	export class Message {
	    id: string;
	    topicId: string;
	    parentId?: string;
	    authorKind: string;
	    participantId?: string;
	    body: string;
	    mentions?: string[];
	    runId?: string;
	    roundId?: string;
	    visibility: string;
	    correctsId?: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.topicId = source["topicId"];
	        this.parentId = source["parentId"];
	        this.authorKind = source["authorKind"];
	        this.participantId = source["participantId"];
	        this.body = source["body"];
	        this.mentions = source["mentions"];
	        this.runId = source["runId"];
	        this.roundId = source["roundId"];
	        this.visibility = source["visibility"];
	        this.correctsId = source["correctsId"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class Participant {
	    id: string;
	    workspaceId: string;
	    name: string;
	    provider: string;
	    model?: string;
	    role?: string;
	    effort?: string;
	    maxWords?: number;
	    mode: string;
	    enabled: boolean;
	    profile: string;
	    tools: string;
	
	    static createFrom(source: any = {}) {
	        return new Participant(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workspaceId = source["workspaceId"];
	        this.name = source["name"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.role = source["role"];
	        this.effort = source["effort"];
	        this.maxWords = source["maxWords"];
	        this.mode = source["mode"];
	        this.enabled = source["enabled"];
	        this.profile = source["profile"];
	        this.tools = source["tools"];
	    }
	}
	
	export class Round {
	    id: string;
	    topicId: string;
	    kind: string;
	    status: string;
	    prompt: string;
	    participantIds: string[];
	    inputs: FrozenInputs;
	    autoReveal: boolean;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    revealedAt?: any;
	    // Go type: time
	    finishedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Round(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.topicId = source["topicId"];
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.prompt = source["prompt"];
	        this.participantIds = source["participantIds"];
	        this.inputs = this.convertValues(source["inputs"], FrozenInputs);
	        this.autoReveal = source["autoReveal"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.revealedAt = this.convertValues(source["revealedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
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
	export class Usage {
	    inputTokens: number;
	    outputTokens: number;
	    cachedTokens: number;
	    costUsd: number;
	
	    static createFrom(source: any = {}) {
	        return new Usage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.cachedTokens = source["cachedTokens"];
	        this.costUsd = source["costUsd"];
	    }
	}
	export class Run {
	    id: string;
	    topicId: string;
	    participantId: string;
	    roundId?: string;
	    triggerMessageId?: string;
	    status: string;
	    profile: string;
	    tools: string;
	    isolation: string;
	    workDir?: string;
	    nativeSessionId?: string;
	    resultMessageId?: string;
	    error?: string;
	    usage: Usage;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    startedAt?: any;
	    // Go type: time
	    finishedAt?: any;
	
	    static createFrom(source: any = {}) {
	        return new Run(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.topicId = source["topicId"];
	        this.participantId = source["participantId"];
	        this.roundId = source["roundId"];
	        this.triggerMessageId = source["triggerMessageId"];
	        this.status = source["status"];
	        this.profile = source["profile"];
	        this.tools = source["tools"];
	        this.isolation = source["isolation"];
	        this.workDir = source["workDir"];
	        this.nativeSessionId = source["nativeSessionId"];
	        this.resultMessageId = source["resultMessageId"];
	        this.error = source["error"];
	        this.usage = this.convertValues(source["usage"], Usage);
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.startedAt = this.convertValues(source["startedAt"], null);
	        this.finishedAt = this.convertValues(source["finishedAt"], null);
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
	export class Topic {
	    id: string;
	    workspaceId: string;
	    title: string;
	    kind: string;
	    // Go type: time
	    createdAt: any;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Topic(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.workspaceId = source["workspaceId"];
	        this.title = source["title"];
	        this.kind = source["kind"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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
	    name: string;
	    rootDir: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Workspace(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.rootDir = source["rootDir"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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

export namespace runner {
	
	export class Capabilities {
	    provider: string;
	    streaming: boolean;
	    resume: boolean;
	    cancel: boolean;
	    toolApprovals: boolean;
	    toolRestrictions: boolean;
	    efforts?: string[];
	    profiles: string[];
	    maxIsolation: string;
	
	    static createFrom(source: any = {}) {
	        return new Capabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.streaming = source["streaming"];
	        this.resume = source["resume"];
	        this.cancel = source["cancel"];
	        this.toolApprovals = source["toolApprovals"];
	        this.toolRestrictions = source["toolRestrictions"];
	        this.efforts = source["efforts"];
	        this.profiles = source["profiles"];
	        this.maxIsolation = source["maxIsolation"];
	    }
	}

}

