import Store from './store.js';

function createStore() {
    return new Store({
        actions: {
        },
        mutations: {
        },
        state: {
            version: "",
        }
    });
}

export default createStore;
