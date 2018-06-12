let BigBro = {
    // This should not be modified outside of the init method.
    data: {
        user: "",
        server: "",
        events: ["click", "dblclick", "mousedown", "mouseup",
            "mouseenter", "mouseover", "mouseout", "wheel",
            "loadstart", "loadend", "load", "unload",
            "reset", "submit", "scroll", "resize",
            "cut", "copy", "paste", "select"
        ],
    },
    // init must be called with the user and the server, and optionally a list of
    // events to listen to globally.
    init: function (user, server, events) {
        this.data.user = user;
        this.data.server = server;
        this.data.events = events || this.data.events;

        let protocol = 'ws://';
        if (window.location.protocol === 'https:') {
            protocol = 'wss://';
        }

        this.ws = new WebSocket(protocol + this.data.server + "/event");

        let self = this;
        for (let i = 0; i < this.data.events.length; i++) {
            window.addEventListener(this.data.events[i], function (e) {
                self.log(e, self.data.events[i]);
            })
        }
        return this;
    },
    // log
    log: function (e, method) {
        let event = {
            target: e.target.tagName,
            name: e.target.name,
            id: e.target.id,
            method: method,
            location: window.location.href,
            time: new Date().toISOString(),
            x: e.x,
            y: e.y,
            screenWidth: window.innerWidth,
            screenHeight: window.innerHeight,
            actor: {
                identifier: this.data.user
            },
            comment: ""
        };
        if (method === "keydown" || method === "keyup") {
            // Which key was actually pressed?
            event.comment = e.code;
        }
        if (method === "paste" || method === "cut" || method === "copy") {
            // Seems like we can only get data for paste events.
            event.comment = e.clipboardData.getData("text/plain")
        }
        if (method === "wheel") {
            // Strength of the wheel rotation.
            event.comment = e.deltaY.toString();
        }

        event.comment = event.comment.replace(/(?:\r\n|\r|\n)/g, "\\n");
        this.ws.send(JSON.stringify(event));
    }
};