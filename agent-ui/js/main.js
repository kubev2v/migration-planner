import createStore from './store/index.js';
import FormView from './views/form.js';


const formContainerElement = document.querySelector("#form-container");
const btnContainerElement = document.querySelector("#btn-container");

const store = createStore();

// Set things up
document.addEventListener('DOMContentLoaded', function() {
    new FormView(store, formContainerElement, btnContainerElement).render();
    store.dispatch('fetchAgentState');
    store.dispatch('fetchAgentVersion');
    store.dispatch('fetchPlannerUrl');
});
