let BigBro = {
    // This should not be modified outside of the init method.
    data: {
        user: "",
        server: "",
        events: ["click", "dblclick", "mousedown", "mouseup",
            "mouseenter", "mouseout", "wheel", "loadstart", "loadend", "load",
            "unload", "reset", "submit", "scroll", "resize",
            "cut", "copy", "paste", "select", "keydown", "keyup"
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

        this.ws.onopen = function (ev) {
            for (let i = 0; i < self.data.events.length; i++) {
                window.addEventListener(self.data.events[i], function (e) {
                    self.log(e, self.data.events[i]);
                })
            }
            bb.log(ev, "bigbroinit");
        };

        window.onunload = function (ev) {
            bb.log(ev, "bigbrodeinit");
            self.ws.close();
            console.log("hello");
        };
        return this
    },
    // log logs an event with a specified method name (normally the actual event name).
    log: function (e, method, comment) {
        if (this.ws.readyState !== 1) {
            console.warn("bigbro websocket unable to connect");
            return false;
        }
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
            actor: this.data.user
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
        if (comment != null) {
            event.comment = comment;
        }
        this.ws.send(JSON.stringify(event));
        return true
    }
};