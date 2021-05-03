pipeline {
  environment {
    CI = 1
    BUILDKIT_PROGRESS = plain
    DOCKER_BUILDKIT = 1
  }
  options {
    timeout(time: 1, unit: 'HOURS')
    timestamps()
    ansiColor('xterm')
  }
  stages {
    stage('checkout') {
      checkout([
        $class                           : 'GitSCM',
        branches                         : scm.branches,
        doGenerateSubmoduleConfigurations: scm.doGenerateSubmoduleConfigurations,
        extensions                       : scm.extensions + [[$class: 'CloneOption', noTags: false, reference: '', shallow: false]],
        submoduleCfg                     : [],
        userRemoteConfigs                : scm.userRemoteConfigs
        ])
    }
    stage('build builder') {
      sh """
        docker build -f mage.dockerfile --output=type=local,dest=. .
      """
    }
    stage('clean') {
      sh "./builder clean"
    }
    // stage('lint') {
    //   sh "./builder test:lint"
    // }
    stage('unit-test') {
      sh "./builder test:unit"
    }
    stage('build') {
      // TODO(dima): build robotest image
      sh "./builder cluster:gravity"
    }
    // TODO(dima): add test/robotest stages
  }
}

////
// Original Jenkinsfile: start
////
/*
#!/usr/bin/env groovy
def propagateParamsToEnv() {
  for (param in params) {
    if (env."${param.key}" == null) {
      env."${param.key}" = param.value
    }
  }
}

/*
// Define Robotest configuration parameters that may be tweaked per job.
// This is needed for the Jenkins GitHub Branch Source Plugin
// which creases a unique Jenkins job for each pull request.
def setRobotestParameters() {
  properties([
      disableConcurrentBuilds(),
      parameters([
        // WARNING: changing parameters will not affect the next build, only the following one
        // see issue #1315 or https://stackoverflow.com/questions/46680573/ -- 2020-04 walt
        choice(choices: ["skip", "run"].join("\n"),
          // defaultValue is not applicable to choices. The first choice will be the default.
          description: 'Run or skip robotest system wide tests.',
          name: 'RUN_ROBOTEST'),
        choice(choices: ["true", "false"].join("\n"),
          description: 'Destroy all VMs on success.',
          name: 'DESTROY_ON_SUCCESS'),
        choice(choices: ["true", "false"].join("\n"),
          description: 'Destroy all VMs on failure.',
          name: 'DESTROY_ON_FAILURE'),
        choice(choices: ["true", "false"].join("\n"),
          description: 'Abort all tests upon first failure.',
          name: 'FAIL_FAST'),
      ]),
  ])
}

// robotest() defines the robotest pipeline.  It is expected to be run after
// the gravity build, as it implicitly relies on artifacts & state from those targets.
//
// Per https://plugins.jenkins.io/pipeline-stage-view/: If we want to visualize
// dynamically changing stages, it is better to make it conditional to execute the
// stage contents, not conditional to include the stage.
def robotest() {
  stage('set robotest params') {
    // For try builds, we DO NOT want to overwrite parameters, as try builds
    // offer a superset of PR/nightly parameters, and the extra ones will be
    // lost when setRobotestParameters() is called -- 2020-04 walt
    echo "Jenkins Job Parameters:"
    for(int i = 0; i < params.size(); i++) { echo "${params[i]}" }
    if (env.KEEP_PARAMETERS == 'true') {
      echo "KEEP_PARAMETERS detected. Ignoring Jenkins job parameters from Jenkinsfile."
    } else {
      echo "Overwriting Jenkins job parameters with parameters from Jenkinsfile."
      setRobotestParameters()
      propagateParamsToEnv()
    }
  }
  runRobotest = (env.RUN_ROBOTEST == 'run')
  stage('build robotest images') {
    if (runRobotest) {
      // Use a shared cache outside the build directory, to avoid repeat downloads and improve
      // build time. For more info see:
      //   https://github.com/gravitational/gravity/blob/4c7ac3ada1e3fb50cf8afdd1d1a4ed4d34bb75d0/assets/robotest/README.md#local-caching
      withEnv(["ROBOTEST_CACHE_ROOT=/var/lib/gravity/robotest-cache"]) {
        sh 'make -C e/assets/robotest images'
      }
    } else {
      echo 'skipping building robotest images'
    }
  }
  throttle(['robotest']) {
    stage('run robotest') {
      if (runRobotest) {
        withCredentials([
          file(credentialsId:'ROBOTEST_LOG_GOOGLE_APPLICATION_CREDENTIALS', variable: 'GOOGLE_APPLICATION_CREDENTIALS'),
          file(credentialsId:'OPS_SSH_KEY', variable: 'SSH_KEY'),
          file(credentialsId:'OPS_SSH_PUB', variable: 'SSH_PUB'),
        ]) {
          sh 'make -C e robotest-run'
        }
      } else {
        echo 'skipping robotest execution'
      }
    }
  } // end throttle
}
*/
////
// Original Jenkinsfile: end
////
