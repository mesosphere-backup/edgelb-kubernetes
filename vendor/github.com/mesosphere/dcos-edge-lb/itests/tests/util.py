import json
import logging
import os
import random
import shlex
import shutil
import string
import subprocess
import sys
import time

import retrying

DEFAULT_TIMEOUT = 360
DEFAULT_WAIT = 3


class ExampleFile(object):
    def __init__(self, d, f):
        self.src = "{}/{}/{}".format(exampledir(), d, f)
        self.dst = "{}/{}{}".format(workdir(), rand_str(5), f)

    def __enter__(self):
        shutil.copyfile(self.src, self.dst)
        return self.dst

    def __exit__(self, t, value, traceback):
        os.remove(self.dst)


class ExampleApp(ExampleFile):
    def __init__(self, f):
        return super().__init__("apps", f)


class ExampleConfig(ExampleFile):
    def __init__(self, f):
        return super().__init__("config", f)


class ExampleTemplate(ExampleFile):
    def __init__(self, f):
        return super().__init__("templates", f)


def get_logger(name):
    return logging.getLogger(name)


def configure_logger(log_level):
    logging.basicConfig(
        format=('%(threadName)s: '
                '%(asctime)s '
                '%(pathname)s:%(funcName)s:%(lineno)d - '
                '%(message)s'),
        stream=sys.stderr,
        level=log_level)


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def wait_marathon():
    logger.info("Waiting for marathon deployments")
    o = sh("dcos marathon deployment list",
           check=False,
           stdout=open(os.devnull, 'w'),
           verbose=False)
    assert o.returncode != 0


def wipe_marathon_things(avoid_edgelb=True):
    wipe_marathon_apps(avoid_edgelb=avoid_edgelb)
    wipe_marathon_pods(avoid_edgelb=avoid_edgelb)


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def wipe_marathon_apps(avoid_edgelb=True):
    o = sh("dcos marathon app list --json", stdout=subprocess.PIPE)
    j = json.loads(o.stdout.decode())
    appids = [app["id"] for app in j]

    for aid in appids:
        if avoid_edgelb:
            if aid.startswith("/dcos-edgelb/"):
                continue
        sh("dcos marathon app remove --force {}".format(aid))

    wait_marathon()


def kill_marathon_apps(ids):
    for i in ids:
        sh("dcos marathon app remove --force {}".format(i))

    wait_marathon()


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def wipe_marathon_pods(avoid_edgelb=True):
    o = sh("dcos marathon pod list --json", stdout=subprocess.PIPE)
    j = json.loads(o.stdout.decode())
    appids = [app["id"] for app in j]

    for aid in appids:
        if avoid_edgelb:
            if aid.startswith("/dcos-edgelb/"):
                continue
        sh("dcos marathon pod remove --force {}".format(aid))

    wait_marathon()


def kill_marathon_pods(ids):
    for i in ids:
        sh("dcos marathon pod remove --force {}".format(i))

    wait_marathon()


def task_up(taskid, exact=False):
    return wait_task_existence(taskid, exact=exact, exists=True)


def task_down(taskid, exact=True):
    return wait_task_existence(taskid, exact=exact, exists=False)


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def wait_task_existence(taskid, exact=False, exists=True):
    """
    Return the task JSON if the task is expected to exist.
    Return None if expecting task to not exist.
    """

    logger.info("waiting for (exact {}) task {} existence to be {}".format(
          exact, taskid, exists))
    o = sh("dcos task --json", check=False, stdout=subprocess.PIPE)
    tasks = json.loads(o.stdout.decode())
    for t in tasks:
        tid = t['id']
        logger.info("found task {}".format(tid))
        if ((exact and tid == taskid) or
                (not exact and tid.startswith(taskid))):
            assert exists
            return t
    assert not exists
    return None


@retrying.retry(wait_fixed=DEFAULT_WAIT * 1000,
                stop_max_delay=DEFAULT_TIMEOUT * 1000,
                retry_on_exception=lambda x: True)
def retry_sh(cmd, verbose=True, shell=False, check=True, stdout=None,
             stderr=None, cwd=None, timer=False):
    return sh(cmd, verbose=verbose, shell=shell, check=check, stdout=stdout,
              stderr=stderr, cwd=cwd, timer=timer)


def sh(cmd, verbose=True, shell=False, check=True, stdout=None, stderr=None,
       cwd=None, timer=False):

    origcmd = cmd

    starttime = time.time()

    if verbose:
        logger.info(cmd)

    if not shell:
        cmd = shlex.split(cmd)
        if verbose:
            logger.info(cmd)

    o = subprocess.run(cmd,
                       shell=shell,
                       check=check,
                       stdout=stdout,
                       stderr=stderr,
                       cwd=cwd)

    endtime = time.time()
    if timer:
        logger.info("{} took:\n{} seconds".format(origcmd, endtime-starttime))

    return o


def workdir():
    env_name = "PYTEST_WORKDIR"
    d = os.getenv(env_name)
    if d is None:
        raise ValueError("{} must be set".format(env_name))
    return d


def exampledir():
    env_name = "EXAMPLES_DIR"
    d = os.getenv(env_name)
    if d is None:
        raise ValueError("{} must be set".format(env_name))
    return d


def rand_str(n):
    chars = string.ascii_lowercase + string.digits
    return ''.join(random.choice(chars) for _ in range(n))


def uninstall_repos():
    o = sh("dcos package repo list", stdout=subprocess.PIPE)
    for line in o.stdout.decode().splitlines():
        reponame = line.split()[0].strip(":")
        if reponame.lower() == "universe":
            continue
        sh("dcos package repo remove {}".format(reponame))


logger = get_logger(__name__)
