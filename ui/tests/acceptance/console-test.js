import { module, test } from 'qunit';
import { settled, waitUntil } from '@ember/test-helpers';
import { create } from 'ember-cli-page-object';
import { setupApplicationTest } from 'ember-qunit';
import enginesPage from 'vault/tests/pages/secrets/backends';
import authPage from 'vault/tests/pages/auth';
import consoleClass from 'vault/tests/pages/components/console/ui-panel';

const consoleComponent = create(consoleClass);

module('Acceptance | console', function (hooks) {
  setupApplicationTest(hooks);

  hooks.beforeEach(function () {
    return authPage.login();
  });

  test("refresh reloads the current route's data", async function (assert) {
    await enginesPage.visit();
    await settled();
    let numEngines = enginesPage.rows.length;
    await consoleComponent.toggle();
    await settled();
    let now = Date.now();
    for (let num of [1, 2, 3]) {
      let inputString = `write sys/mounts/${now + num} type=kv`;
      await consoleComponent.runCommands(inputString);
      await settled();
    }
    await consoleComponent.runCommands('refresh');
    await settled();
    assert.equal(enginesPage.rows.length, numEngines + 3, 'new engines were added to the page');
  });

  test('fullscreen command expands the cli panel', async function (assert) {
    await consoleComponent.toggle();
    await settled();
    await consoleComponent.runCommands('fullscreen');
    await settled();
    const consoleEle = document.querySelector('[data-test-component="console/ui-panel"]');
    // wait for the CSS transition to finish
    await waitUntil(() => consoleEle.offsetHeight === window.innerHeight);
    assert.equal(consoleEle.offsetHeight, window.innerHeight, 'fullscreen is the same height as the window');
    assert.equal(consoleEle.offsetTop, 0, 'fullscreen is aligned to the top of window');
  });

  test('array output is correctly formatted', async function (assert) {
    await consoleComponent.toggle();
    await settled();
    await consoleComponent.runCommands('read -field=policies /auth/token/lookup-self');
    await settled();
    const consoleOut = document.querySelector('.console-ui-output>pre');
    // wait for the CSS transition to finish
    await waitUntil(() => consoleOut.innerText);
    assert.notOk(consoleOut.innerText.includes('function(){'));
    assert.equal(consoleOut.innerText, '["root"]');
  });

  test('number output is correctly formatted', async function (assert) {
    await consoleComponent.toggle();
    await settled();
    await consoleComponent.runCommands('read -field=creation_time /auth/token/lookup-self');
    await settled();
    const consoleOut = document.querySelector('.console-ui-output>pre');
    // wait for the CSS transition to finish
    await waitUntil(() => consoleOut.innerText);
    assert.ok(consoleOut.innerText.match(/^\d+$/).length == 1);
  });

  test('boolean output is correctly formatted', async function (assert) {
    await consoleComponent.toggle();
    await settled();
    await consoleComponent.runCommands('read -field=orphan /auth/token/lookup-self');
    await settled();
    let consoleOut = document.querySelector('.console-ui-output>pre');
    // have to wrap in a later so that we can wait for the CSS transition to finish
    await waitUntil(() => consoleOut.innerText);
    assert.ok(consoleOut.innerText.match(/^(true|false)$/g).length == 1);
  });
});
