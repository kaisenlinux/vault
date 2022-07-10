import Component from '@ember/component';

export default Component.extend({
  'data-test-component': 'console/output-log',
  attributeBindings: ['data-test-component'],
  log: null,
});
