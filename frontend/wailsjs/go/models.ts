export namespace main {
	
	export class FileNode {
	    name: string;
	    path: string;
	    relPath: string;
	    rootPath: string;
	    rootLabel: string;
	    sizeBytes: number;
	    tokensApprox: number;
	    isDir: boolean;
	    children?: FileNode[];
	    isGitignored: boolean;
	    isCustomIgnored: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileNode(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.relPath = source["relPath"];
	        this.rootPath = source["rootPath"];
	        this.rootLabel = source["rootLabel"];
	        this.sizeBytes = source["sizeBytes"];
	        this.tokensApprox = source["tokensApprox"];
	        this.isDir = source["isDir"];
	        this.children = this.convertValues(source["children"], FileNode);
	        this.isGitignored = source["isGitignored"];
	        this.isCustomIgnored = source["isCustomIgnored"];
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
	export class LLMSettings {
	    activeProvider: string;
	    model: string;
	    openAIKey: string;
	    openRouterKey: string;
	    geminiKey: string;
	    baseURL: string;
	
	    static createFrom(source: any = {}) {
	        return new LLMSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.activeProvider = source["activeProvider"];
	        this.model = source["model"];
	        this.openAIKey = source["openAIKey"];
	        this.openRouterKey = source["openRouterKey"];
	        this.geminiKey = source["geminiKey"];
	        this.baseURL = source["baseURL"];
	    }
	}
	export class ProjectRoot {
	    path: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectRoot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.label = source["label"];
	    }
	}
	export class PromptHistoryItem {
	    id: string;
	    // Go type: time
	    timestamp: any;
	    userTask: string;
	    constructedPrompt: string;
	    response: string;
	    apiCall?: string;
	
	    static createFrom(source: any = {}) {
	        return new PromptHistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.userTask = source["userTask"];
	        this.constructedPrompt = source["constructedPrompt"];
	        this.response = source["response"];
	        this.apiCall = source["apiCall"];
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

export namespace provider {
	
	export class ModelInfo {
	    name: string;
	    description?: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	    }
	}

}

