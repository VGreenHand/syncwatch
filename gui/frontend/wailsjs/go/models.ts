export namespace main {
	
	export class Config {
	    server: string;
	    name: string;
	    lastVideo: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.server = source["server"];
	        this.name = source["name"];
	        this.lastVideo = source["lastVideo"];
	    }
	}

}

