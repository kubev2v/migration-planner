import Store from '../store/store.js';

export default class Observable {
    defaultEvent = "stateChange";

    constructor(props = {}) {
        let self = this;

        this.render = this.render || function() { };

        // If there's a store passed in, subscribe to the state change
        if (props.store instanceof Store) {
            if (props.hasOwnProperty('event')) {
                props.store.events.subscribe(props.event, () => self.render());
            } else {
                props.store.events.subscribe(this.defaultEvent, () => self.render());
            }
            this.store = props.store;
        }

        // Store the HTML element to attach the render to if set
        if (props.hasOwnProperty('element')) {
            this.element = props.element;
        }
    }
}
